package usage

import (
	"context"
	"errors"
	"time"

	"github.com/linuxea/item-system/domain/behavior"
	"github.com/linuxea/item-system/domain/effect"
	"github.com/linuxea/item-system/domain/event"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/repository"
)

var (
	ErrInstanceNotFound = errors.New("instance not found")
	ErrNotOwner         = errors.New("instance does not belong to owner")
	ErrNotUsable        = errors.New("item is not usable")
	ErrNotAvailable     = errors.New("item is not available")
	ErrInsufficient     = errors.New("insufficient item count")
	ErrIdempotency      = errors.New("duplicate idempotency key")
)

type Clock func() time.Time

type Request struct {
	Owner          string
	InstanceID     string
	Count          int64
	IdempotencyKey string
	Params         map[string]any
}

type Service struct {
	templates   repository.TemplateSource
	instances   repository.InstanceRepo
	idempotency repository.IdempotencyStore
	publisher   event.Publisher
	registry    *behavior.Registry
	runtime     effect.Runtime
	now         Clock
}

func NewService(
	templates repository.TemplateSource,
	instances repository.InstanceRepo,
	idempotency repository.IdempotencyStore,
	publisher event.Publisher,
	registry *behavior.Registry,
	runtime effect.Runtime,
	now Clock,
) *Service {
	return &Service{
		templates:   templates,
		instances:   instances,
		idempotency: idempotency,
		publisher:   publisher,
		registry:    registry,
		runtime:     runtime,
		now:         now,
	}
}

func (s *Service) Use(ctx context.Context, req Request) (*model.UseResult, error) {
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.IdempotencyKey != "" {
		ok, err := s.idempotency.Claim(ctx, req.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrIdempotency
		}
	}

	inst, err := s.instances.Get(ctx, req.InstanceID)
	if err != nil {
		s.release(ctx, req.IdempotencyKey)
		return nil, err
	}
	if inst.Owner != req.Owner {
		s.release(ctx, req.IdempotencyKey)
		return nil, ErrNotOwner
	}
	if !inst.Available(s.now()) {
		s.release(ctx, req.IdempotencyKey)
		return nil, ErrNotAvailable
	}

	tpl, err := s.templates.Get(ctx, inst.TemplateID)
	if err != nil {
		s.release(ctx, req.IdempotencyKey)
		return nil, err
	}
	compiled, err := s.registry.Compile(tpl)
	if err != nil {
		s.release(ctx, req.IdempotencyKey)
		return nil, err
	}
	usable, ok := compiled.Usable()
	if !ok {
		s.release(ctx, req.IdempotencyKey)
		return nil, ErrNotUsable
	}
	if inst.Count < req.Count {
		s.release(ctx, req.IdempotencyKey)
		return nil, ErrInsufficient
	}

	cmds, err := effect.Materialize(usable.Effects, req.Params)
	if err != nil {
		s.release(ctx, req.IdempotencyKey)
		return nil, err
	}
	rt := s.runtime
	rt.Owner = req.Owner
	for i := int64(0); i < req.Count; i++ {
		if err := effect.Execute(ctx, rt, cmds); err != nil {
			s.release(ctx, req.IdempotencyKey)
			return nil, err
		}
	}

	expect := inst.Version
	inst.Count -= req.Count
	inst.BumpVersion()
	if inst.Count == 0 {
		if err := s.instances.Delete(ctx, inst.ID); err != nil {
			return nil, err
		}
	} else if err := s.instances.Update(ctx, inst, expect); err != nil {
		return nil, err
	}

	s.publisher.Publish(event.Consumed{
		Base:       event.Base{At: s.now()},
		Owner:      req.Owner,
		TemplateID: inst.TemplateID,
		InstanceID: inst.ID,
		Count:      req.Count,
	})
	return &model.UseResult{InstanceID: inst.ID, Consumed: req.Count}, nil
}

func (s *Service) release(ctx context.Context, key string) {
	if key == "" {
		return
	}
	_ = s.idempotency.Release(ctx, key)
}
