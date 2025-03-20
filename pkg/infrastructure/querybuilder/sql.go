package querybuilder

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

type SQL struct {
}

func NewSQL() *SQL {
	return &SQL{}
}

func (s *SQL) BuildSQLQuery(prometheusQuery *prompb.Query) (string, error) {
	matchers := make([]string, 0, len(prometheusQuery.Matchers))
	labelEqualPredicates := make(map[string]string)

	for _, m := range prometheusQuery.Matchers {
		escapedName := escapeValue(m.Name)
		escapedValue := escapeValue(m.Value)

		if m.Name == model.MetricNameLabel {
			switch m.Type {
			case prompb.LabelMatcher_EQ:
				if len(escapedValue) == 0 {
					matchers = append(matchers, fmt.Sprintf("(l.metric_name IS NULL OR name = '')"))
				} else {
					matchers = append(matchers, fmt.Sprintf("l.metric_name = '%s'", escapedValue))
				}
			case prompb.LabelMatcher_NEQ:
				matchers = append(matchers, fmt.Sprintf("l.metric_name != '%s'", escapedValue))
			case prompb.LabelMatcher_RE:
				matchers = append(matchers, fmt.Sprintf("l.metric_name ~ '%s'", anchorValue(escapedValue)))
			case prompb.LabelMatcher_NRE:
				matchers = append(matchers, fmt.Sprintf("l.metric_name !~ '%s'", anchorValue(escapedValue)))
			default:
				return "", fmt.Errorf("unknown metric name match type %v", m.Type)
			}
		} else {
			switch m.Type {
			case prompb.LabelMatcher_EQ:
				if len(escapedValue) == 0 {
					// From the PromQL docs: "Label matchers that match
					// empty label values also select all time series that
					// do not have the specific label set at all."
					matchers = append(matchers, fmt.Sprintf("((l.metric_labels ? '%s') = false OR (l.metric_labels->>'%s' = ''))",
						escapedName, escapedName))
				} else {
					labelEqualPredicates[escapedName] = escapedValue
				}
			case prompb.LabelMatcher_NEQ:
				matchers = append(matchers, fmt.Sprintf("l.metric_labels->>'%s' != '%s'", escapedName, escapedValue))
			case prompb.LabelMatcher_RE:
				matchers = append(matchers, fmt.Sprintf("l.metric_labels->>'%s' ~ '%s'", escapedName, anchorValue(escapedValue)))
			case prompb.LabelMatcher_NRE:
				matchers = append(matchers, fmt.Sprintf("l.metric_labels->>'%s' !~ '%s'", escapedName, anchorValue(escapedValue)))
			default:
				return "", fmt.Errorf("unknown match type %v", m.Type)
			}
		}
	}
	equalsPredicate := ""

	if len(labelEqualPredicates) > 0 {
		labelsJSON, err := json.Marshal(labelEqualPredicates)

		if err != nil {
			return "", err
		}
		equalsPredicate = fmt.Sprintf(" AND l.metric_labels @> '%s'", labelsJSON)
	}

	matchers = append(matchers, fmt.Sprintf("v.metric_time >= '%v'", toTimestamp(prometheusQuery.StartTimestampMs).Format(time.RFC3339)))
	matchers = append(matchers, fmt.Sprintf("v.metric_time <= '%v'", toTimestamp(prometheusQuery.EndTimestampMs).Format(time.RFC3339)))

	return fmt.Sprintf("SELECT v.metric_time, l.metric_name, v.metric_value, l.metric_labels FROM metric_values v, metric_labels l WHERE l.metric_id = v.metric_id and %s %s ORDER BY v.metric_time",
		strings.Join(matchers, " AND "), equalsPredicate), nil
}

func escapeValue(str string) string {
	return strings.Replace(str, `'`, `''`, -1)
}

// anchorValue adds anchors to values in regexps since PromQL docs
// states that "Regex-matches are fully anchored."
func anchorValue(str string) string {
	l := len(str)

	if l == 0 || (str[0] == '^' && str[l-1] == '$') {
		return str
	}

	if str[0] == '^' {
		return fmt.Sprintf("%s$", str)
	}

	if str[l-1] == '$' {
		return fmt.Sprintf("^%s", str)
	}

	return fmt.Sprintf("^%s$", str)
}

func toTimestamp(milliseconds int64) time.Time {
	sec := milliseconds / 1000
	nsec := (milliseconds - (sec * 1000)) * 1000000
	return time.Unix(sec, nsec).UTC()
}
