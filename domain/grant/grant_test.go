package grant_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linuxea/item-system/domain/behavior"
	"github.com/linuxea/item-system/domain/grant"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/repository"
	"github.com/linuxea/item-system/infrastructure/memory"
)

func newStack(tpls ...*model.ItemTemplate) *memory.Stack {
	return memory.NewStack(tpls...)
}

func potion() *model.ItemTemplate {
	return &model.ItemTemplate{
		ID:       "potion_hp",
		Category: model.CategoryConsumable,
		Name:     "HP Potion",
		Behaviors: map[string]map[string]any{
			behavior.KeyStackable: {"max_stack": int64(99)},
		},
	}
}

func TestGrantStacking(t *testing.T) {
	ctx := context.Background()
	s := newStack(potion())

	r1, err := s.App.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "potion_hp", Count: 5})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	r2, err := s.App.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "potion_hp", Count: 3})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if r1.InstanceID != r2.InstanceID {
		t.Fatalf("expected merge into same instance, got %s and %s", r1.InstanceID, r2.InstanceID)
	}
	instances, _ := s.Instances.ListByOwner(ctx, "p1")
	if len(instances) != 1 || instances[0].Count != 8 {
		t.Fatalf("expected single stack of 8, got %+v", instances)
	}
}

func TestGrantOverflowCreatesNewInstance(t *testing.T) {
	ctx := context.Background()
	s := newStack(potion())

	if _, err := s.App.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "potion_hp", Count: 99}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if _, err := s.App.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "potion_hp", Count: 5}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	instances, _ := s.Instances.ListByOwner(ctx, "p1")
	if len(instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(instances))
	}
	if instances[0].Count != 99 || instances[1].Count != 5 {
		t.Fatalf("unexpected counts: %+v", instances)
	}
}

func TestGrantIdempotency(t *testing.T) {
	ctx := context.Background()
	s := newStack(potion())

	req := grant.Request{Owner: "p1", TemplateID: "potion_hp", Count: 1, IdempotencyKey: "op-1"}
	if _, err := s.App.Grant(ctx, req); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if _, err := s.App.Grant(ctx, req); !errors.Is(err, grant.ErrIdempotency) {
		t.Fatalf("expected ErrIdempotency, got %v", err)
	}
	instances, _ := s.Instances.ListByOwner(ctx, "p1")
	if len(instances) != 1 || instances[0].Count != 1 {
		t.Fatalf("expected single unit, got %+v", instances)
	}
}

func TestGrantUnknownTemplate(t *testing.T) {
	ctx := context.Background()
	s := newStack(potion())

	var notFound *repository.ErrNotFound
	_, err := s.App.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "nope", Count: 1})
	if !errors.As(err, &notFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGrantExpirable(t *testing.T) {
	ctx := context.Background()
	s := newStack(&model.ItemTemplate{
		ID:       "vip_month",
		Category: model.CategoryVIP,
		Name:     "VIP Month",
		Behaviors: map[string]map[string]any{
			behavior.KeyExpirable: {"duration": "720h", "on_expire": behavior.ExpirePolicyDowngrade, "downgrade_to": "vip_normal"},
		},
	})
	before := time.Now().Add(-time.Hour)
	if _, err := s.App.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "vip_month", Count: 1, ExpireAt: &before}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	instances, _ := s.Instances.ListByOwner(ctx, "p1")
	if len(instances) != 1 || instances[0].ExpireAt == nil || !instances[0].ExpireAt.Before(time.Now()) {
		t.Fatalf("expected expired instance, got %+v", instances)
	}
}
