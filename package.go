package letitgo

import (
	"context"
)

type Startable interface {
	StartWithContext(ctx context.Context, replyTo chan<- error)
}

type Stopable interface {
	StopWithContext(ctx context.Context, why error, replyTo chan<- error)
}

type Health = int

const (
	Green = iota
	Yellow
	Red
)

type Healthy interface {
	Health(ctx context.Context) <-chan Health
}

type Service interface {
	Startable
	Stopable
	Healthy
	ID() string
}
