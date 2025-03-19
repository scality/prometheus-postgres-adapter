package domain

import (
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
)
