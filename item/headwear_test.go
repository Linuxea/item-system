// headwear_test.go 头饰：穿戴与被动光效、限时款到期仅卸下保留实例。
package item_test

import (
	"context"
	"testing"
	"time"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/item/catalog"
	"github.com/Linuxea/item-system/memory"
)

func headwearStack(t *testing.T) (*item.Inventory, *memory.Stack, *time.Time) {
	t.Helper()
	inv, s, clock := newStack(t)
	s.Defs.Register(catalog.Headwears()...)
	return inv, s, clock
}

func TestHeadwearEquipAndProfile(t *testing.T) {
	ctx := context.Background()
	inv, _, _ := headwearStack(t)

	id := grantAndEquip(t, inv, "p1", "crown_aurora")

	snap, err := inv.Build(ctx, "p1", "profile_home")
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	hw := snap.Slots[item.SlotHeadwear]
	if len(hw) != 1 || hw[0].DefID != "crown_aurora" {
		t.Fatalf("headwear slot wrong: %+v", hw)
	}
	if len(hw[0].Modifiers) != 1 || hw[0].Modifiers[0].Key != "head_glow" {
		t.Fatalf("headwear modifiers wrong: %+v", hw[0].Modifiers)
	}

	if err := inv.Unequip(ctx, "p1", id); err != nil {
		t.Fatalf("unequip: %v", err)
	}
	snap, _ = inv.Build(ctx, "p1", "profile_home")
	if len(snap.Slots[item.SlotHeadwear]) != 0 {
		t.Fatalf("headwear slot should be empty")
	}
}

func TestHeadwearExpirableUnequipKeepsItem(t *testing.T) {
	ctx := context.Background()
	inv, s, clock := headwearStack(t)

	id := grantAndEquip(t, inv, "p1", "cap_sprint_3d")

	forceExpire(t, s, id, clock.Add(-time.Second))
	n, err := inv.RunExpiry(ctx, 10)
	if err != nil || n != 1 {
		t.Fatalf("expiry run: n=%d err=%v", n, err)
	}

	inst, err := s.Instances.Get(ctx, id)
	if err != nil {
		t.Fatalf("instance should survive unequip policy: %v", err)
	}
	if inst.Status != item.StatusNormal || inst.ExpireAt == nil {
		t.Fatalf("unexpected instance state: %+v", inst)
	}

	snap, _ := inv.Build(ctx, "p1", "profile_home")
	if len(snap.Slots[item.SlotHeadwear]) != 0 {
		t.Fatalf("slot should be empty after expiry unequip")
	}

	var seen bool
	for _, e := range s.Bus.Recorded() {
		if ev, ok := e.(item.Expired); ok && ev.InstanceID == id {
			seen = true
			if ev.Policy != item.PolicyUnequip {
				t.Fatalf("wrong policy: %s", ev.Policy)
			}
		}
	}
	if !seen {
		t.Fatalf("expected expiry event")
	}
}
