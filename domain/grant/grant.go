package grant

import (
	"context"
	"errors"
	"time"

	"github.com/linuxea/item-system/domain/behavior"
	"github.com/linuxea/item-system/domain/event"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/repository"
)

var (
	ErrInvalidCount     = errors.New("grant count must be positive")
	ErrTemplateNotFound = errors.New("template not found")
	ErrIdempotency      = errors.New("duplicate idempotency key")
)

type Clock func() time.Time

type Request struct {
	Owner          string
	TemplateID     string
	Count          int64
	Source         string
	Reason         string
	IdempotencyKey string
	ExpireAt       *time.Time
}

type Service struct {
	templates   repository.TemplateSource
	instances   repository.InstanceRepo
	idempotency repository.IdempotencyStore
	publisher   event.Publisher
	registry    *behavior.Registry
	newID       func() string
	now         Clock
}

func NewService(
	templates repository.TemplateSource,
	instances repository.InstanceRepo,
	idempotency repository.IdempotencyStore,
	publisher event.Publisher,
	registry *behavior.Registry,
	newID func() string,
	now Clock,
) *Service {
	return &Service{
		templates:   templates,
		instances:   instances,
		idempotency: idempotency,
		publisher:   publisher,
		registry:    registry,
		newID:       newID,
		now:         now,
	}
}

func (s *Service) Grant(ctx context.Context, req Request) (*model.GrantResult, error) {
	if req.Count <= 0 {
		return nil, ErrInvalidCount
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

	tpl, err := s.templates.Get(ctx, req.TemplateID)
	if err != nil {
		s.release(ctx, req.IdempotencyKey)
		return nil, err
	}
	compiled, err := s.registry.Compile(tpl)
	if err != nil {
		s.release(ctx, req.IdempotencyKey)
		return nil, err
	}

	primaryID, err := s.apply(ctx, req, compiled)
	if err != nil {
		s.release(ctx, req.IdempotencyKey)
		return nil, err
	}

	s.publisher.Publish(event.Granted{
		Base:       event.Base{At: s.now()},
		Owner:      req.Owner,
		TemplateID: req.TemplateID,
		InstanceID: primaryID,
		Count:      req.Count,
		Source:     req.Source,
		Reason:     req.Reason,
	})
	return &model.GrantResult{InstanceID: primaryID, Count: req.Count}, nil
}

func (s *Service) apply(ctx context.Context, req Request, compiled *behavior.Compiled) (string, error) {
	now := s.now()

	stack, _ := compiled.Stackable()
	expirable, hasExpiry := compiled.Expirable()
	bindable, _ := compiled.Bindable()

	remain := req.Count
	var primaryID string

	if stack.MaxStack > 1 {
		existing, err := s.instances.ListByOwner(ctx, req.Owner)
		if err != nil {
			return "", err
		}
		for _, inst := range existing {
			if remain == 0 {
				break
			}
			if !s.canMergeInto(inst, req, stack, now) {
				continue
			}
			room := stack.MaxStack - inst.Count
			if room <= 0 {
				continue
			}
			take := min(remain, room)
			expect := inst.Version
			inst.Count += take
			inst.BumpVersion()
			if err := s.instances.Update(ctx, inst, expect); err != nil {
				return "", err
			}
			remain -= take
			if primaryID == "" {
				primaryID = inst.ID
			}
		}
	}

	if remain > 0 {
		inst := &model.ItemInstance{
			ID:         s.newID(),
			TemplateID: req.TemplateID,
			Owner:      req.Owner,
			Count:      remain,
			Bound:      bindable.BindOnPickup,
			Status:     model.StatusNormal,
			AcquiredAt: now,
			Version:    0,
		}
		switch {
		case req.ExpireAt != nil:
			t := *req.ExpireAt
			inst.ExpireAt = &t
		case hasExpiry && expirable.Duration > 0:
			t := now.Add(expirable.Duration)
			inst.ExpireAt = &t
		}
		if err := s.instances.Save(ctx, inst); err != nil {
			return "", err
		}
		if primaryID == "" {
			primaryID = inst.ID
		}
	}
	return primaryID, nil
}

func (s *Service) canMergeInto(inst *model.ItemInstance, req Request, stack behavior.Stackable, now time.Time) bool {
	if inst.TemplateID != req.TemplateID || !inst.Available(now) {
		return false
	}
	if !sameExpiry(inst.ExpireAt, req.ExpireAt) {
		return false
	}
	return true
}

func sameExpiry(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

func (s *Service) release(ctx context.Context, key string) {
	if key == "" {
		return
	}
	_ = s.idempotency.Release(ctx, key)
}
