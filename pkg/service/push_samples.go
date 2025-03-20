package service

import "prometheus-postgres-adapter/pkg/domain"

type MessageQueuePusher interface {
	Push(samples *domain.Samples) error
}
