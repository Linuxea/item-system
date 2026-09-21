package usage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/linuxea/item-system/domain/behavior"
	"github.com/linuxea/item-system/domain/grant"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/usage"
	"github.com/linuxea/item-system/infrastructure/memory"
)

func badge() *model.ItemTemplate {
	return &model.ItemTemplate{
		ID:       "badge_flame",
		Category: model.CategoryBadge,
		Name:     "Flame Badge",
		Behaviors: map[string]map[string]any{
			behavior.KeyEquippable: {"slot": string(model.SlotBadge), "capacity": 3},
		},
	}
}

func TestUseNotUsable(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStack(badge())

	r, err := s.App.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "badge_flame", Count: 1})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if _, err := s.App.Use(ctx, usage.Request{Owner: "p1", InstanceID: r.InstanceID}); !errors.Is(err, usage.ErrNotUsable) {
		t.Fatalf("expected ErrNotUsable, got %v", err)
	}
}

func TestUseNotOwner(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStack(badge())

	r, err := s.App.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "badge_flame", Count: 1})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if _, err := s.App.Use(ctx, usage.Request{Owner: "p2", InstanceID: r.InstanceID}); !errors.Is(err, usage.ErrNotOwner) {
		t.Fatalf("expected ErrNotOwner, got %v", err)
	}
}
