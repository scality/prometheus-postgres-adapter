//nolint:cyclop,mnd // This package is complex
package querybuilder

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/scality/go-errors"
)

//nolint:gochecknoglobals // The bounds of what a timestamp literal can say.
var (
	// earliestQueryTime and latestQueryTime bound what RFC 3339 can write with
	// four digits of year, well within what PostgreSQL can store.
	earliestQueryTime = time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	latestQueryTime   = time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC)
)

var (
	// ErrUnknownMatchType is returned for an unsupported label matcher type.
	ErrUnknownMatchType = errors.New("unknown match type")
	// ErrUnknownMetricNameMatchType is returned for an unsupported metric name matcher type.
	ErrUnknownMetricNameMatchType = errors.New("unknown metric name match type")
)

const (
	sqlBaseQueryFormat = `
		SELECT v.metric_time, l.metric_name, v.metric_value, l.metric_labels
		FROM metric_values v, metric_labels l
		WHERE l.metric_id = v.metric_id
		  AND %s
		ORDER BY v.metric_time
`

	// noMatcherPredicate is the predicate used when no matcher restricts the
	// selected series.
	noMatcherPredicate = "TRUE"
)

type SQL struct{}

func NewSQL() *SQL {
	return &SQL{}
}

// BuildSQLQuery transforms a Prometheus query into SQL.
// It handles various label matcher types (equality, inequality, regex) by constructing
// appropriate SQL conditions:
// - For metric names (__name__), conditions are applied directly to the metric_name column
// - For equality matchers, conditions use the JSONB containment operator (@>)
// - For regex matchers, conditions use PostgreSQL regex operators (~ and !~)
//
// It anchors regex patterns as a whole and escapes single quotes.
// Time constraints from the query are added as conditions on the metric_time column.
func (*SQL) BuildSQLQuery(prometheusQuery *prompb.Query) (string, error) {
	conditions, err := buildLabelConditions(prometheusQuery.Matchers)
	if err != nil {
		return "", err
	}

	conditions = append(
		conditions,
		fmt.Sprintf("v.metric_time >= '%s'", timestampLiteral(prometheusQuery.StartTimestampMs)),
		fmt.Sprintf("v.metric_time <= '%s'", timestampLiteral(prometheusQuery.EndTimestampMs)),
	)

	return fmt.Sprintf(sqlBaseQueryFormat, strings.Join(conditions, " AND ")), nil
}

// BuildLabelsPredicate turns label matchers into a SQL predicate over the
// metric_labels table, aliased l. Unlike BuildSQLQuery it never touches
// metric_values, so label metadata can be filtered by matchers without joining
// the samples. It returns TRUE when no matcher restricts the series.
func (*SQL) BuildLabelsPredicate(matchers []*prompb.LabelMatcher) (string, error) {
	conditions, err := buildLabelConditions(matchers)
	if err != nil {
		return "", err
	}

	if len(conditions) == 0 {
		return noMatcherPredicate, nil
	}

	return strings.Join(conditions, " AND "), nil
}

// buildLabelConditions turns label matchers into SQL conditions on the
// metric_labels table, aliased l.
//
//nolint:gocognit,funlen,mnd // Query building is complex by essence
func buildLabelConditions(labelMatchers []*prompb.LabelMatcher) ([]string, error) {
	matchers := make([]string, 0, len(labelMatchers))

	for _, m := range labelMatchers {
		escapedName := escapeValue(m.Name)
		escapedValue := escapeValue(m.Value)
		pattern := patternOf(m.Type, escapedValue)

		if m.Name != model.MetricNameLabel {
			switch m.Type {
			case prompb.LabelMatcher_EQ:
				matchers = append(matchers, equalityCondition(escapedName, escapedValue))
			case prompb.LabelMatcher_NEQ:
				matchers = append(
					matchers,
					fmt.Sprintf(
						"%s != '%s'",
						labelValueOrEmpty(escapedName),
						escapedValue,
					),
				)
			case prompb.LabelMatcher_RE:
				matchers = append(
					matchers,
					fmt.Sprintf(
						"%s ~ '%s'",
						labelValueOrEmpty(escapedName),
						pattern,
					),
				)
			case prompb.LabelMatcher_NRE:
				matchers = append(
					matchers,
					fmt.Sprintf(
						"%s !~ '%s'",
						labelValueOrEmpty(escapedName),
						pattern,
					),
				)
			default:
				return nil, errors.Wrap(ErrUnknownMatchType, errors.WithProperty("type", m.Type))
			}

			continue
		}

		switch m.Type {
		case prompb.LabelMatcher_EQ:
			// Bare, like the three below: metric_name is NOT NULL in the schema
			// this reads, so an empty name is an empty string and nothing else.
			// Defending against NULL here only, as this did, made the answer to
			// a NULL name depend on which matcher asked -- selected by an
			// equality, filtered by the other three, since a comparison with
			// NULL is NULL.
			matchers = append(matchers, fmt.Sprintf("l.metric_name = '%s'", escapedValue))
		case prompb.LabelMatcher_NEQ:
			matchers = append(matchers, fmt.Sprintf("l.metric_name != '%s'", escapedValue))
		case prompb.LabelMatcher_RE:
			matchers = append(matchers, fmt.Sprintf("l.metric_name ~ '%s'", pattern))
		case prompb.LabelMatcher_NRE:
			matchers = append(matchers, fmt.Sprintf("l.metric_name !~ '%s'", pattern))
		default:
			return nil, errors.Wrap(ErrUnknownMetricNameMatchType, errors.WithProperty("type", m.Type))
		}
	}

	return matchers, nil
}

func escapeValue(str string) string {
	return strings.ReplaceAll(str, `'`, `''`)
}

// labelValueOrEmpty reads a label off the JSONB column, reporting a series that
// does not carry it as carrying it empty, the way PromQL matches a missing label.
func labelValueOrEmpty(escapedName string) string {
	return fmt.Sprintf("COALESCE(l.metric_labels->>'%s', '')", escapedName)
}

// equalityCondition asks the JSONB column for one label and value, the form the
// GIN index serves, and one condition per matcher: PromQL selects nothing when
// a label is asked to hold two values at once, and two containments that cannot
// both hold say exactly that. Merging them into a single object instead would
// have one value overwrite the other.
//
// From the PromQL docs, "Label matchers that match empty label values also
// select all time series that do not have the specific label set at all", so an
// empty value is a comparison rather than a containment.
func equalityCondition(escapedName, escapedValue string) string {
	if len(escapedValue) == 0 {
		return fmt.Sprintf("%s = ''", labelValueOrEmpty(escapedName))
	}

	// Marshalling a map of strings cannot fail: invalid UTF-8 is replaced
	// rather than refused.
	labelJSON, _ := json.Marshal(map[string]string{escapedName: escapedValue})

	return fmt.Sprintf("l.metric_labels @> '%s'", labelJSON)
}

// patternOf anchors the pattern of a regex matcher. Other matcher types carry
// no pattern.
func patternOf(matcherType prompb.LabelMatcher_Type, escapedValue string) string {
	if matcherType != prompb.LabelMatcher_RE && matcherType != prompb.LabelMatcher_NRE {
		return ""
	}

	return anchorPattern(escapedValue)
}

// anchorPattern anchors a matcher pattern as a whole, the way PromQL does.
// Anchoring by appending ^ and $ to the pattern would bind them to the first
// and last branch of an alternation only: `node|kubelet` would become
// `^node|kubelet$`, which matches `node-legacy` and `old-kubelet`.
func anchorPattern(pattern string) string {
	return fmt.Sprintf("^(?:%s)$", pattern)
}

// timestampLiteral renders a query bound, down to the fraction of a second:
// truncating it would drop the samples of the last second of every range.
//
// Prometheus and Thanos say "no bound" with the extremes of int64
// milliseconds, and those are years RFC 3339 cannot write and PostgreSQL cannot
// store: a request carrying one would fail the whole query, which is what
// /api/v1/series without a time range sends. They are clamped to a range both
// understand instead, which selects the same rows.
func timestampLiteral(milliseconds int64) string {
	timestamp := model.Time(milliseconds).Time().UTC()

	switch {
	case timestamp.Before(earliestQueryTime):
		timestamp = earliestQueryTime
	case timestamp.After(latestQueryTime):
		timestamp = latestQueryTime
	}

	return timestamp.Format(time.RFC3339Nano)
}
