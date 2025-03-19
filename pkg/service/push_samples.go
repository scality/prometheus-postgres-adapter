package service

import "prom-adapter/pkg/domain"

type MessageQueuePusher interface {
	Push(samples *domain.Samples) error
}
