package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/scality/go-errors"
)

var (
	// ErrInvalidLabelsType is returned when a labels value has an unexpected type.
	ErrInvalidLabelsType = errors.New("invalid type for labels")
	// ErrUnmarshalLabels is returned when labels cannot be unmarshalled from JSON.
	ErrUnmarshalLabels = errors.New("failed to unmarshal labels")
)

type (
	Samples struct {
		S model.Samples
	}

	ReadRequest struct {
		R *prompb.ReadRequest
	}

	ReadResponse struct {
		R *prompb.ReadResponse
	}

	SampleLabels struct {
		JSON        []byte
		Map         map[string]string
		OrderedKeys []string
	}

	SamplesReadFromDatabase struct {
		Timestamp time.Time
		Value     float64
		Name      string
		Labels    SampleLabels
	}
)

func (l *SampleLabels) Scan(value any) error {
	if value == nil {
		l = &SampleLabels{} //nolint:ineffassign // Avoid nil pointer dereference
		return nil
	}

	// Example of labels received from Prometheus:
	// {
	//   "job": "federate-prometheus",
	//   "path": "/mnt/data-01",
	//   "endpoint": "http",
	//   "instance": "prometheus-operator-prometheus.metalk8s-monitoring.svc:9090",
	//   "long_term": "true",
	//   "prometheus_replica": "prometheus-prometheus-operator-prometheus-0",
	// }
	var t []byte

	switch v := value.(type) {
	case []uint8:
		t = v
	case string:
		t = []byte(v)
	default:
		return errors.Wrap(ErrInvalidLabelsType, errors.WithProperty("type", fmt.Sprintf("%T", value)))
	}

	m := make(map[string]string)

	err := json.Unmarshal(t, &m)
	if err != nil {
		return errors.Wrap(ErrUnmarshalLabels, errors.CausedBy(err))
	}

	*l = SampleLabels{
		JSON:        t,
		Map:         m,
		OrderedKeys: createOrderedKeys(&m),
	}

	return nil
}

func createOrderedKeys(m *map[string]string) []string {
	keys := make([]string, 0, len(*m))
	for k := range *m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

func (l SampleLabels) String() string {
	return string(l.JSON)
}

func (l SampleLabels) Key(extra string) string {
	// 0xff cannot occur in valid UTF-8 sequences, so use it
	// as a separator here.
	separator := "\xff"
	pairs := make([]string, 0, len(l.Map)+1)
	pairs = append(pairs, extra+separator)

	for _, k := range l.OrderedKeys {
		pairs = append(pairs, k+separator+l.Map[k])
	}

	return strings.Join(pairs, separator)
}
