package messagequeue

import (
	"prometheus-postgres-adapter/pkg/domain"
)

var queue = make(chan *domain.Samples)

type Chan struct {
}

func NewChan() *Chan {
	return &Chan{}
}

func (p *Chan) Push(samples *domain.Samples) error {
	queue <- samples
	return nil
}

func (p *Chan) Pop() *domain.Samples {
	return <-queue
}
