package letitgo

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	"sync"
)

type ProtocolViolationError struct {
	reason string
}

func (pve *ProtocolViolationError) Error() string {
	return fmt.Sprintf("protocol violated: %s", pve.reason)
}

type startWithContextParams struct {
	ctx     context.Context
	replyTo chan<- error
}

type stopWithContextParams struct {
	ctx     context.Context
	why     error
	replyTo chan<- error
}

type healthParams struct {
	ctx     context.Context
	replyTo chan<- Health
}

type healthyServiceProtocol struct {
	startWithContext chan startWithContextParams
	stopWithContext  chan stopWithContextParams
	health           chan healthParams
}

type state = bool

type healthyService struct {
	l *log.Logger

	service Service

	id string

	history      []*Health
	historySize  int
	historyIndex int

	reducer func([]Health) Health

	state state

	p healthyServiceProtocol

	once sync.Once
}

func (hs *healthyService) debug(msg string) {
	hs.l.WithFields(log.Fields{
		"id": hs.ID(),
	}).Debug(msg)
}

func (hs *healthyService) reset() {
	hs.debug(fmt.Sprintf("%s reset", hs.ID()))

	for i, _ := range hs.history {
		hs.history[i] = nil
	}
	hs.historyIndex = -1

	hs.state = false

	select {
	case <-hs.p.startWithContext:
		hs.debug("startWithContext reset")
	default:
	}

	select {
	case <-hs.p.stopWithContext:
		hs.debug("stopWithContext reset")
	default:
	}

	select {
	case <-hs.p.health:
		hs.debug("health reset")
	default:
	}

	hs.once = sync.Once{}
}

func (hs *healthyService) copyHistory() []Health {
	if hs.historyIndex == -1 {
		return make([]Health, 0)
	}

	i := (hs.historyIndex + 1) % hs.historySize
	if hs.history[i] == nil {
		res := make([]Health, i)

		for j, _ := range res {
			res[j] = *hs.history[j]
		}

		return res
	} else {
		res := make([]Health, hs.historySize)
		res[0] = *hs.history[i]

		n := 1
		for n < hs.historySize {
			i = (i + 1) % hs.historySize

			res[n] = *hs.history[i]

			n += 1
		}

		return res
	}
}

func (hs *healthyService) loop() {
	for {
		select {
		case req := <-hs.p.startWithContext:
			select {
			case <-req.ctx.Done():
				hs.debug(fmt.Sprintf("StartWithContext: %s", req.ctx.Err().Error()))

				req.replyTo <- req.ctx.Err()

				continue
			default:
			}

			if hs.state {
				hs.debug("StartWithContext: already started")

				req.replyTo <- nil

				continue
			}

			hs.debug(fmt.Sprintf("StartWithContext: starting a %s", hs.service.ID()))
			replyTo := make(chan error)

			ctx, cancel := context.WithCancel(req.ctx)
			hs.service.StartWithContext(ctx, replyTo)

			var err error
			select {
			case <-req.ctx.Done():
				cancel()
				err = req.ctx.Err()

			case err = <-replyTo:
			}

			if err == nil {
				hs.debug("StartWithContext: started normally")

				hs.state = true
			} else {
				hs.debug(
					fmt.Sprintf(
						"StartWithContext: %s started with non-nil error %s",
						hs.service.ID(),
						err.Error()))
			}

			req.replyTo <- err

		case req := <-hs.p.stopWithContext:
			select {
			case <-req.ctx.Done():
				hs.debug(fmt.Sprintf("StopWithContext: %s", req.ctx.Err().Error()))

				req.replyTo <- req.ctx.Err()

				continue
			default:
			}

			if !hs.state {
				hs.debug("StopWithContext: already stopped")

				req.replyTo <- nil

				continue
			}

			hs.debug(fmt.Sprintf("StopWithContext: stopping a %s", hs.service.ID()))
			replyTo := make(chan error)
			ctx, cancel := context.WithCancel(req.ctx)
			hs.service.StopWithContext(ctx, req.why, replyTo)

			var err error
			select {
			case <-req.ctx.Done():
				cancel()
				err = req.ctx.Err()

			case err = <-replyTo:
			}

			if err == nil {
				hs.debug("StopWithContext: stopped normally")
				hs.reset()
			} else {
				hs.debug(
					fmt.Sprintf(
						"StopWithContext: %s stopped with non-nil error %s",
						hs.service.ID(),
						err.Error()))
			}

			req.replyTo <- err

		case req := <-hs.p.health:
			select {
			case <-req.ctx.Done():
				hs.debug(fmt.Sprintf("Health: red because %s", req.ctx.Err().Error()))

				req.replyTo <- Red

				continue
			default:
			}

			if !hs.state {
				hs.debug("Health: red because service is not started")

				req.replyTo <- Red

				continue
			}

			var health Health

			ctx, cancel := context.WithCancel(req.ctx)
			select {
			case <-req.ctx.Done():
				cancel()
				health = Red

			case health = <-hs.service.Health(ctx):
			}

			i := (hs.historyIndex + 1) % hs.historySize
			hs.history[i] = &health
			hs.historyIndex = i

			req.replyTo <- hs.reducer(hs.copyHistory())
		}
	}
}

func (hs *healthyService) ID() string {
	return hs.id
}

func (hs *healthyService) StartWithContext(ctx context.Context, replyTo chan<- error) {
	hs.once.Do(func() { go hs.loop() })

	hs.debug("StartWithContext: request sent")

	hs.p.startWithContext <- startWithContextParams{
		ctx:     ctx,
		replyTo: replyTo,
	}
}

func (hs *healthyService) StopWithContext(ctx context.Context, why error, replyTo chan<- error) {
	hs.once.Do(func() { go hs.loop() })

	hs.debug("StopWithContext: request sent")

	hs.p.stopWithContext <- stopWithContextParams{
		ctx:     ctx,
		replyTo: replyTo,
	}
}

func (hs *healthyService) Health(ctx context.Context) <-chan Health {
	hs.once.Do(func() { go hs.loop() })

	res := make(chan Health)

	hs.debug("Health: request sent")

	hs.p.health <- healthParams{
		ctx:     ctx,
		replyTo: res,
	}

	return res
}

func AnyRedOrYellowReducer(history []Health) Health {
	seenYellow := false
	for _, health := range history {
		switch health {
		case Yellow:
			seenYellow = true
		case Red:
			return Red
		default:
			continue
		}
	}

	if seenYellow {
		return Yellow
	}

	return Green
}

func newHealthyService(logger *log.Logger, service Service, historySize uint16, reducer func([]Health) Health) (error, *healthyService) {
	if service == nil {
		return fmt.Errorf("service can not be nil"), nil
	}

	if historySize == 0 {
		return fmt.Errorf("historySize can not be zero"), nil
	}

	if reducer == nil {
		reducer = AnyRedOrYellowReducer
	}

	res := healthyService{
		l: logger,

		service: service,

		id: fmt.Sprintf("healthService-%s", service.ID()),

		history:      make([]*Health, int(historySize)),
		historySize:  int(historySize),
		historyIndex: -1,

		reducer: reducer,

		state: false,

		p: healthyServiceProtocol{
			startWithContext: make(chan startWithContextParams),
			stopWithContext:  make(chan stopWithContextParams),
			health:           make(chan healthParams),
		},

		once: sync.Once{},
	}

	return nil, &res
}

func NewHealthyService(logger *log.Logger, service Service, historySize uint16, reducer func([]Health) Health) (error, Service) {
	return newHealthyService(logger, service, historySize, reducer)
}
