package usecase

import "prom-adapter/pkg/domain"

type WriteMetricsQueueContent struct {
	queue *chan *domain.Samples
}
