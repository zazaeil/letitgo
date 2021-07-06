package letitgo

import (
	"context"
	"math/rand"
	"testing"
	"time"
)

func Test_HealthyService_GreenYellowRed(t *testing.T) {
	ts := newTestService()

	err, s := NewHealthyService(newTestLogger(), ts, 1, nil)
	if err != nil {
		t.FailNow()
	}

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

	ts.mockHealth(Yellow)
	if h := <-s.Health(context.TODO()); h != Yellow {
		t.FailNow()
	}

	ts.mockHealth(Red)
	if h := <-s.Health(context.TODO()); h != Red {
		t.FailNow()
	}

	ts.mockHealth(Yellow)
	if h := <-s.Health(context.TODO()); h != Yellow {
		t.FailNow()
	}

	ts.mockHealth(Green)
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

func Test_HealthyService_Idempotency(t *testing.T) {
	ts := newTestService()

	err, s := NewHealthyService(newTestLogger(), ts, 1, nil)
	if err != nil {
		t.FailNow()
	}

	if h := <-s.Health(context.TODO()); h != Red {
		t.FailNow()
	}

	replyTo := make(chan error)

	j := rand.Intn(1_000)

	for i := 0; i < j; i++ {
		s.StartWithContext(context.TODO(), replyTo)
		if err := <-replyTo; err != nil {
			t.FailNow()
		}

		if h := <-s.Health(context.TODO()); h != Green {
			t.FailNow()
		}
	}

	for i := 0; i < j; i++ {
		s.StopWithContext(context.TODO(), nil, replyTo)
		if err := <-replyTo; err != nil {
			t.FailNow()
		}

		if h := <-s.Health(context.TODO()); h != Red {
			t.FailNow()
		}
	}
}

func Test_HealthyService_Restartable(t *testing.T) {
	ts := newTestService()

	err, s := NewHealthyService(newTestLogger(), ts, 1, nil)
	if err != nil {
		t.FailNow()
	}

	if h := <-s.Health(context.TODO()); h != Red {
		t.FailNow()
	}

	replyTo := make(chan error)

	j := rand.Intn(1_000)

	for i := 0; i < j; i++ {
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
}

func Test_HealthyService_StartWithContext_RespectsContext(t *testing.T) {
	ts := newTestServiceWith(
		func(context.Context, chan<- error) {
			for {
			}
		},
		nil,
		nil)

	err, s := NewHealthyService(newTestLogger(), ts, 1, nil)
	if err != nil {
		t.FailNow()
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*time.Duration(rand.Intn(50)))
	defer cancel()

	replyTo := make(chan error)
	s.StartWithContext(ctx, replyTo)
	select {
	case err := <-replyTo:
		if err == nil {
			t.FailNow()
		}

	case <-time.After(time.Second):
		t.FailNow()
	}
}

func Test_HealthyService_Health_RespectsContext(t *testing.T) {
	ts := newTestService()

	err, s := NewHealthyService(newTestLogger(), ts, 1, nil)
	if err != nil {
		t.FailNow()
	}

	replyTo := make(chan error)

	s.StartWithContext(context.TODO(), replyTo)
	if err := <-replyTo; err != nil {
		t.FailNow()
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*time.Duration(rand.Intn(50)))
	defer cancel()

	ts.mockHealthWith(func(context.Context) Health {
		for {
		}

		return Red
	})

	if h := <-s.Health(ctx); h != Red {
		t.FailNow()
	}
}

func Test_HealthyService_StopWithContext_RespectsContext(t *testing.T) {
	ts := newTestService()

	err, s := NewHealthyService(newTestLogger(), ts, 1, nil)
	if err != nil {
		t.FailNow()
	}

	replyTo := make(chan error)
	s.StartWithContext(context.TODO(), replyTo)
	if err := <-replyTo; err != nil {
		t.FailNow()
	}

	ts.stopWithContext = func(context.Context, error, chan<- error) {
		for {
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*time.Duration(rand.Intn(50)))
	defer cancel()

	s.StopWithContext(ctx, nil, replyTo)
	select {
	case err := <-replyTo:
		if err == nil {
			t.FailNow()
		}

	case <-time.After(time.Second):
		t.FailNow()
	}
}

func historyToString(history []Health) string {
	res := "["

	for _, health := range history {
		switch health {
		case Green:
			res += "G"
		case Yellow:
			res += "Y"
		case Red:
			res += "R"
		}
	}

	return res + "]"
}

func Test_HealthyService_HistoryIsCircularAndPreservesOrder(t *testing.T) {
	ts := newTestService()

	err, s := newHealthyService(newTestLogger(), ts, 3, nil)
	if err != nil {
		t.FailNow()
	}

	if len(s.copyHistory()) > 0 {
		t.FailNow()
	}

	replyTo := make(chan error)
	s.StartWithContext(context.TODO(), replyTo)
	if err := <-replyTo; err != nil {
		t.FailNow()
	}

	if len(s.copyHistory()) > 0 {
		t.Errorf("History must be empty.")
		t.FailNow()
	}

	<-s.Health(context.TODO())
	var actual = s.copyHistory()
	if len(actual) != 1 || actual[0] != Green {
		t.Errorf(
			"Histroy must contain exactly one ``G`` fact, but was %s.", historyToString(actual))
		t.FailNow()
	}

	<-s.Health(context.TODO())
	actual = s.copyHistory()
	if len(actual) != 2 || actual[0] != Green || actual[1] != Green {
		t.Errorf("History must contain exactly two ``G`` facts, %v.", actual)

		t.FailNow()
	}

	<-s.Health(context.TODO())
	actual = s.copyHistory()
	if len(actual) != 3 || actual[0] != Green || actual[1] != Green || actual[2] != Green {
		t.Errorf("History must contain exactly three ``G`` facts.")
		t.FailNow()
	}

	ts.mockHealth(Yellow)

	<-s.Health(context.TODO())
	actual = s.copyHistory()
	if actual[0] != Green || actual[1] != Green || actual[2] != Yellow {
		t.Errorf("History must preserve order: expected [GGY], but was %s.", historyToString(actual))
		t.FailNow()
	}

	<-s.Health(context.TODO())
	actual = s.copyHistory()
	if actual[0] != Green || actual[1] != Yellow || actual[2] != Yellow {
		t.FailNow()
	}

	<-s.Health(context.TODO())
	actual = s.copyHistory()
	if actual[0] != Yellow || actual[1] != Yellow || actual[2] != Yellow {
		t.FailNow()
	}

	ts.mockHealth(Red)

	<-s.Health(context.TODO())
	actual = s.copyHistory()
	if actual[0] != Yellow || actual[1] != Yellow || actual[2] != Red {
		t.FailNow()
	}

	<-s.Health(context.TODO())
	actual = s.copyHistory()
	if actual[0] != Yellow || actual[1] != Red || actual[2] != Red {
		t.FailNow()
	}

	<-s.Health(context.TODO())
	actual = s.copyHistory()
	if actual[0] != Red || actual[1] != Red || actual[2] != Red {
		t.FailNow()
	}

	s.StopWithContext(context.TODO(), nil, replyTo)
	if err := <-replyTo; err != nil {
		t.FailNow()
	}

	if len(s.copyHistory()) > 0 {
		t.FailNow()
	}
}
