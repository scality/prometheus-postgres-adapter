package messagequeue

import (
	"prometheus-postgres-adapter/pkg/domain"
)

type Chan struct {
	queue chan *domain.Samples
}

func NewChan() *Chan {
	return &Chan{
		queue: make(chan *domain.Samples),
	}
}

func (p *Chan) Push(samples *domain.Samples) error {
	p.queue <- samples
	return nil
}

func (p *Chan) Pop() *domain.Samples {
	return <-p.queue
}
