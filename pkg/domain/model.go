package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
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
	//   "pod": "artesca-storage-service-e848f788-ds-0",
	//   "path": "/mnt/data-01",
	//   "service": "artesca-storage-service-ds-e848f788",
	//   "endpoint": "http",
	//   "instance": "prometheus-operator-prometheus.metalk8s-monitoring.svc:9090",
	//   "container": "hd",
	//   "long_term": "true",
	//   "namespace": "xcore",
	//   "prometheus": "metalk8s-monitoring/prometheus-operator-prometheus",
	//   "exported_job": "artesca-storage-service-ds-e848f788",
	//   "prometheus_replica": "prometheus-prometheus-operator-prometheus-0",
	//   "xcore_scality_com_node_name": "ip-10-0-129-137.eu-north-1.compute.internal",
	//   "xcore_scality_com_resource_type": "dataserver"
	// }
	var t []byte
	switch v := value.(type) {
	case []uint8:
		t = v
	case string:
		t = []byte(v)
	default:
		return fmt.Errorf("invalid type for labels: %T", value)
	}

	m := make(map[string]string)
	err := json.Unmarshal(t, &m)

	if err != nil {
		return err
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
