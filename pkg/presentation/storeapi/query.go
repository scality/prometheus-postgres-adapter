// Package storeapi implements a Thanos StoreAPI gRPC server backed by the
// PostgreSQL adapter, allowing Thanos to query long-term samples directly
// without going through a Prometheus remote_read passthrough.
package storeapi

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/prometheus/prometheus/prompb"
	"github.com/scality/go-errors"
	"github.com/thanos-io/thanos/pkg/store/storepb"
)

// ErrUnsupportedLabelMatcherType is returned for an unsupported StoreAPI label matcher type.
var ErrUnsupportedLabelMatcherType = errors.New("unsupported label matcher type")

//nolint:gochecknoglobals // Static lookup table for matcher type translation.
var matcherTypeToProm = map[storepb.LabelMatcher_Type]prompb.LabelMatcher_Type{
	storepb.LabelMatcher_EQ:  prompb.LabelMatcher_EQ,
	storepb.LabelMatcher_NEQ: prompb.LabelMatcher_NEQ,
	storepb.LabelMatcher_RE:  prompb.LabelMatcher_RE,
	storepb.LabelMatcher_NRE: prompb.LabelMatcher_NRE,
}

//nolint:gochecknoglobals // Static lookup table for matcher rendering.
var matcherTypeToOperator = map[storepb.LabelMatcher_Type]string{
	storepb.LabelMatcher_EQ:  "=",
	storepb.LabelMatcher_NEQ: "!=",
	storepb.LabelMatcher_RE:  "=~",
	storepb.LabelMatcher_NRE: "!~",
}

// loggableMatchers renders request matchers for logging, in the PromQL form
// Thanos uses, and only when a handler takes the record: slog resolves a
// LogValuer at that point, so nothing is rendered at a level that discards it.
//
// It stands in for storepb.MatchersToString, which panics on a matcher type the
// enum does not know. proto3 enums are open, so any client can send one, and a
// request is logged before it is validated.
type loggableMatchers []storepb.LabelMatcher

func (m loggableMatchers) LogValue() slog.Value {
	rendered := make([]string, 0, len(m))

	for _, matcher := range m {
		operator, known := matcherTypeToOperator[matcher.Type]
		if !known {
			operator = fmt.Sprintf("<%d>", matcher.Type)
		}

		rendered = append(rendered, fmt.Sprintf("%s%s%q", matcher.Name, operator, matcher.Value))
	}

	return slog.StringValue("{" + strings.Join(rendered, ", ") + "}")
}

// PromQueryFromSeriesRequest converts a Thanos StoreAPI SeriesRequest into a
// prompb.Query so the existing SQL query builder can be reused.
//
// Matchers that target one of the store's external labels are validated against
// the configured value and then dropped: external labels are not stored in the
// database, they are applied to the results, so they must not reach the SQL
// builder. The returned boolean is false when such a matcher excludes this
// store entirely (its external label value does not satisfy the matcher), in
// which case the store has no series to return.
func PromQueryFromSeriesRequest(
	req *storepb.SeriesRequest,
	externalLabels map[string]string,
) (*prompb.Query, bool, error) {
	matchers, matched, err := promMatchers(req.Matchers, externalLabels)
	if err != nil || !matched {
		return nil, matched, err
	}

	return &prompb.Query{
		StartTimestampMs: req.MinTime,
		EndTimestampMs:   req.MaxTime,
		Matchers:         matchers,
	}, true, nil
}

// promMatchers converts StoreAPI matchers into Prometheus ones, validating and
// dropping those targeting an external label. The boolean is false when such a
// matcher excludes this store entirely.
func promMatchers(
	storeMatchers []storepb.LabelMatcher,
	externalLabels map[string]string,
) ([]*prompb.LabelMatcher, bool, error) {
	matchers := make([]*prompb.LabelMatcher, 0, len(storeMatchers))

	for _, m := range storeMatchers {
		if value, ok := externalLabels[m.Name]; ok {
			matches, err := externalLabelMatches(m, value)
			if err != nil {
				return nil, false, err
			}

			if !matches {
				return nil, false, nil
			}

			continue
		}

		promType, ok := matcherTypeToProm[m.Type]
		if !ok {
			return nil, false, errors.Wrap(ErrUnsupportedLabelMatcherType, errors.WithProperty("type", m.Type))
		}

		matchers = append(matchers, &prompb.LabelMatcher{
			Type:  promType,
			Name:  m.Name,
			Value: m.Value,
		})
	}

	return matchers, true, nil
}

// externalLabelMatches reports whether the store's external label value
// satisfies the given matcher (handling regex matchers correctly).
func externalLabelMatches(matcher storepb.LabelMatcher, value string) (bool, error) {
	promMatchers, err := storepb.MatchersToPromMatchers(matcher)
	if err != nil {
		return false, errors.Wrap(ErrUnsupportedLabelMatcherType, errors.CausedBy(err))
	}

	return promMatchers[0].Matches(value), nil
}
