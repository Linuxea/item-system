package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linuxea/item-system/application"
	"github.com/linuxea/item-system/domain/behavior"
	"github.com/linuxea/item-system/domain/event"
	"github.com/linuxea/item-system/domain/grant"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/profile"
	"github.com/linuxea/item-system/domain/usage"
	"github.com/linuxea/item-system/infrastructure/memory"
)

type fixedRand struct{ n int64 }

func (f *fixedRand) next(int64) int64 { return f.n }

func templates() []*model.ItemTemplate {
	return []*model.ItemTemplate{
		{
			ID: "avatar_gold", Category: model.CategoryAvatar, Name: "Gold Avatar", Priority: 90, Rarity: 5,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotAvatar), "capacity": 1},
				behavior.KeyPassive:    {"modifiers": []any{map[string]any{"key": "frame_color", "value": "gold"}}},
			},
		},
		{
			ID: "badge_abyss", Category: model.CategoryBadge, Name: "Abyss Badge", Priority: 70, Rarity: 5,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotBadge), "capacity": 3},
			},
		},
		{
			ID: "badge_flame", Category: model.CategoryBadge, Name: "Flame Badge", Priority: 70, Rarity: 3,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotBadge), "capacity": 3},
			},
		},
		{
			ID: "vip_month", Category: model.CategoryVIP, Name: "VIP Month", Priority: 60, Rarity: 4,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotVIP), "capacity": 1},
				behavior.KeyExpirable:  {"duration": "720h", "on_expire": behavior.ExpirePolicyDowngrade, "downgrade_to": "vip_normal"},
				behavior.KeyPassive:    {"modifiers": []any{map[string]any{"key": "vip_level", "value": float64(3)}}},
			},
		},
		{
			ID: "vip_normal", Category: model.CategoryVIP, Name: "VIP Normal", Priority: 10, Rarity: 1,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotVIP), "capacity": 1},
				behavior.KeyPassive:    {"modifiers": []any{map[string]any{"key": "vip_level", "value": float64(0)}}},
			},
		},
		{
			ID: "potion_hp", Category: model.CategoryConsumable, Name: "HP Potion", Priority: 0, Rarity: 1,
			Behaviors: map[string]map[string]any{
				behavior.KeyStackable: {"max_stack": int64(99)},
				behavior.KeyUsable: {"effects": []any{
					map[string]any{"kind": "add_currency", "currency": "hp", "amount": int64(50)},
				}},
			},
		},
		{
			ID: "chest_basic", Category: model.CategoryConsumable, Name: "Basic Chest", Priority: 0, Rarity: 2,
			Behaviors: map[string]map[string]any{
				behavior.KeyUsable: {"effects": []any{
					map[string]any{"kind": "random_grant", "entries": []any{
						map[string]any{"template_id": "potion_hp", "count": int64(3), "weight": int64(70)},
						map[string]any{"template_id": "badge_flame", "count": int64(1), "weight": int64(30)},
					}},
				}},
			},
		},
	}
}

func newTestApp(t *testing.T) (*application.App, *memory.Stack, *fixedRand, *time.Time) {
	t.Helper()
	now := time.Now()
	clock := &now
	rnd := &fixedRand{n: 0}

	s := memory.NewStack(templates()...)
	bus := s.Bus
	app := application.New(application.Deps{
		Templates:   s.Templates,
		Instances:   s.Instances,
		Equips:      s.Equips,
		Idempotency: s.Idempotency,
		Publisher:   bus,
		Ledger:      s.Ledger,
		NewID:       memory.NewIDGenerator("inst_").Next,
		Now:         func() time.Time { return *clock },
		Rand:        rnd.next,
	})
	s.App = app
	return app, s, rnd, clock
}

func TestFullLifecycle(t *testing.T) {
	ctx := context.Background()
	app, s, rnd, clock := newTestApp(t)

	potion1, err := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "potion_hp", Count: 5})
	if err != nil {
		t.Fatalf("grant potion: %v", err)
	}
	potion2, err := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "potion_hp", Count: 3})
	if err != nil {
		t.Fatalf("grant potion: %v", err)
	}
	if potion1.InstanceID != potion2.InstanceID {
		t.Fatalf("potions should stack into one instance")
	}
	avatar, _ := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "avatar_gold", Count: 1})
	abyss, _ := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "badge_abyss", Count: 1})
	flame, _ := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "badge_flame", Count: 1})
	vip, _ := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "vip_month", Count: 1})

	if err := app.Equip(ctx, "p1", avatar.InstanceID); err != nil {
		t.Fatalf("equip avatar: %v", err)
	}
	if err := app.Equip(ctx, "p1", abyss.InstanceID); err != nil {
		t.Fatalf("equip abyss: %v", err)
	}
	if err := app.Equip(ctx, "p1", flame.InstanceID); err != nil {
		t.Fatalf("equip flame: %v", err)
	}
	if err := app.Equip(ctx, "p1", vip.InstanceID); err != nil {
		t.Fatalf("equip vip: %v", err)
	}

	if err := app.Equip(ctx, "p1", potion1.InstanceID); !errors.Is(err, profile.ErrNotEquippable) {
		t.Fatalf("expected ErrNotEquippable, got %v", err)
	}

	snap, err := app.BuildProfile(ctx, "p1", "profile_home")
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	badges := snap.Slots[model.SlotBadge]
	if len(badges) != 2 || badges[0].TemplateID != "badge_abyss" || badges[1].TemplateID != "badge_flame" {
		t.Fatalf("badge order wrong: %+v", badges)
	}
	if len(snap.Slots[model.SlotAvatar]) != 1 || snap.Slots[model.SlotAvatar][0].TemplateID != "avatar_gold" {
		t.Fatalf("avatar slot wrong: %+v", snap.Slots[model.SlotAvatar])
	}
	if len(snap.Modifiers) != 2 {
		t.Fatalf("expected avatar+vip modifiers, got %+v", snap.Modifiers)
	}

	if _, err := app.Use(ctx, usage.Request{Owner: "p1", InstanceID: potion1.InstanceID, Count: 2}); err != nil {
		t.Fatalf("use potion: %v", err)
	}
	if got := s.Ledger.Balance("p1", "hp"); got != 100 {
		t.Fatalf("expected hp=100, got %d", got)
	}
	inst, _ := s.Instances.Get(ctx, potion1.InstanceID)
	if inst.Count != 6 {
		t.Fatalf("expected 6 potions left, got %d", inst.Count)
	}

	chest, _ := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "chest_basic", Count: 1})
	rnd.n = 0
	if _, err := app.Use(ctx, usage.Request{Owner: "p1", InstanceID: chest.InstanceID}); err != nil {
		t.Fatalf("use chest: %v", err)
	}
	inst, _ = s.Instances.Get(ctx, potion1.InstanceID)
	if inst.Count != 9 {
		t.Fatalf("expected chest to grant 3 potions (6+3), got %d", inst.Count)
	}
	rnd.n = 71
	chest2, _ := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "chest_basic", Count: 1})
	if _, err := app.Use(ctx, usage.Request{Owner: "p1", InstanceID: chest2.InstanceID}); err != nil {
		t.Fatalf("use chest2: %v", err)
	}
	flames, _ := s.Instances.ListByOwner(ctx, "p1")
	flameCount := int64(0)
	for _, i := range flames {
		if i.TemplateID == "badge_flame" {
			flameCount += i.Count
		}
	}
	if flameCount != 2 {
		t.Fatalf("expected 2 flame badges, got %d", flameCount)
	}

	*clock = clock.Add(800 * time.Hour)
	n, err := app.RunExpiry(ctx, 100)
	if err != nil || n != 1 {
		t.Fatalf("expiry run: n=%d err=%v", n, err)
	}
	vipInst, err := s.Instances.Get(ctx, vip.InstanceID)
	if err != nil {
		t.Fatalf("vip instance should survive downgrade: %v", err)
	}
	if vipInst.TemplateID != "vip_normal" || vipInst.Status != model.StatusEquipped || vipInst.ExpireAt != nil {
		t.Fatalf("downgrade wrong: %+v", vipInst)
	}

	snap2, _ := app.BuildProfile(ctx, "p1", "profile_home")
	if got := snap2.Slots[model.SlotVIP][0].TemplateID; got != "vip_normal" {
		t.Fatalf("expected vip_normal in slot after expiry, got %s", got)
	}

	var expiredSeen bool
	for _, e := range s.Bus.Recorded() {
		if ev, ok := e.(event.Expired); ok && ev.InstanceID == vip.InstanceID {
			expiredSeen = true
			if ev.Policy != behavior.ExpirePolicyDowngrade {
				t.Fatalf("wrong expiry policy: %s", ev.Policy)
			}
		}
	}
	if !expiredSeen {
		t.Fatalf("expected ItemExpired event")
	}

	if _, err := app.Use(ctx, usage.Request{Owner: "p1", InstanceID: potion1.InstanceID, Count: 99}); !errors.Is(err, usage.ErrInsufficient) {
		t.Fatalf("expected ErrInsufficient, got %v", err)
	}
}
