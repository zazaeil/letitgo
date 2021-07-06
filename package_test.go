package letitgo

import (
	"context"
	log "github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"testing"
)

type testService struct {
	health           func(context.Context) Health
	startWithContext func(context.Context, chan<- error)
	stopWithContext  func(context.Context, error, chan<- error)
}

func (mock *testService) ID() string {
	return "testService"
}

func (mock *testService) StartWithContext(ctx context.Context, replyTo chan<- error) {
	if mock.startWithContext != nil {
		go func() { mock.startWithContext(ctx, replyTo) }()

		return
	}

	mock.health = func(context.Context) Health { return Green }

	go func() { replyTo <- nil }()
}

func (mock *testService) StopWithContext(ctx context.Context, why error, replyTo chan<- error) {
	if mock.stopWithContext != nil {
		go func() { mock.stopWithContext(ctx, why, replyTo) }()

		return
	}

	mock.health = func(context.Context) Health { return Red }

	go func() { replyTo <- why }()
}

func (mock *testService) Health(ctx context.Context) <-chan Health {
	res := make(chan Health)

	go func() {
		res <- mock.health(ctx)
		close(res)
	}()

	return res
}

func (mock *testService) mockHealth(health Health) {
	mock.health = func(context.Context) Health { return health }
}

func (mock *testService) mockHealthWith(health func(context.Context) Health) {
	mock.health = health
}

func newTestService() *testService {
	res := testService{
		health: func(context.Context) Health { return Red },
	}

	return &res
}

func newTestServiceWith(
	startWithContext func(context.Context, chan<- error),
	stopWithContext func(context.Context, error, chan<- error),
	health func(context.Context) Health) *testService {
	if health == nil {
		health = func(context.Context) Health { return Red }
	}

	res := testService{
		startWithContext: startWithContext,
		stopWithContext:  stopWithContext,
		health:           health,
	}

	return &res
}

func TestSanity(t *testing.T) {
	s := newTestService()

	if h := <-s.Health(context.TODO()); h != Red {
		t.FailNow()
	}

	replyTo := make(chan error)
	s.StartWithContext(context.TODO(), replyTo)
	if err := <-replyTo; err != nil {
		t.FailNow()
	}

	if h := <-s.Health(context.TODO()); h != Green {
		t.FailNow()
	}

	s.StopWithContext(context.TODO(), nil, replyTo)
	if err := <-replyTo; err != nil {
		t.FailNow()
	}

	if h := <-s.Health(context.TODO()); h != Red {
		t.FailNow()
	}
}

func newTestLogger() *log.Logger {
	logger, _ := logTest.NewNullLogger()

	return logger
}
