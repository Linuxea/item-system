package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/linuxea/item-system/catalog"
	"github.com/linuxea/item-system/domain/event"
	"github.com/linuxea/item-system/domain/model"
)

func TestHeadwearEquipAndProfile(t *testing.T) {
	ctx := context.Background()
	app, _, _ := newStackWithClock(t, catalog.Headwear())

	id := grantAndEquip(t, app, "p1", "crown_aurora")

	snap, err := app.BuildProfile(ctx, "p1", "profile_home")
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	hw := snap.Slots[model.SlotHeadwear]
	if len(hw) != 1 || hw[0].TemplateID != "crown_aurora" {
		t.Fatalf("headwear slot wrong: %+v", hw)
	}
	if len(hw[0].Modifiers) != 1 || hw[0].Modifiers[0].Key != "head_glow" {
		t.Fatalf("headwear modifiers wrong: %+v", hw[0].Modifiers)
	}

	if err := app.Unequip(ctx, "p1", id); err != nil {
		t.Fatalf("unequip: %v", err)
	}
	snap, _ = app.BuildProfile(ctx, "p1", "profile_home")
	if len(snap.Slots[model.SlotHeadwear]) != 0 {
		t.Fatalf("headwear slot should be empty")
	}
}

func TestHeadwearExpirableUnequipKeepsItem(t *testing.T) {
	ctx := context.Background()
	app, s, clock := newStackWithClock(t, catalog.Headwear())

	id := grantAndEquip(t, app, "p1", "cap_sprint_3d")

	forceExpire(t, s, id, clock.Add(-time.Second))
	n, err := app.RunExpiry(ctx, 10)
	if err != nil || n != 1 {
		t.Fatalf("expiry run: n=%d err=%v", n, err)
	}

	inst, err := s.Instances.Get(ctx, id)
	if err != nil {
		t.Fatalf("instance should survive unequip policy: %v", err)
	}
	if inst.Status != model.StatusNormal || inst.ExpireAt == nil {
		t.Fatalf("unexpected instance state: %+v", inst)
	}

	snap, _ := app.BuildProfile(ctx, "p1", "profile_home")
	if len(snap.Slots[model.SlotHeadwear]) != 0 {
		t.Fatalf("slot should be empty after expiry unequip")
	}

	var seen bool
	for _, e := range s.Bus.Recorded() {
		if ev, ok := e.(event.Expired); ok && ev.InstanceID == id {
			seen = true
			if ev.Policy != "unequip" {
				t.Fatalf("wrong policy: %s", ev.Policy)
			}
		}
	}
	if !seen {
		t.Fatalf("expected expiry event")
	}
}
