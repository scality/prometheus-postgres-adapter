package domain

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
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

	v, ok := value.([]uint8)
	if !ok {
		return errors.Errorf("failed to scan labels: %s", reflect.TypeOf(value))
	}

	m := make(map[string]string)

	err := json.Unmarshal(v, &m)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal labels")
	}

	*l = SampleLabels{
		JSON:        v,
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
