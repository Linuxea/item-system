// nameplate_test.go 铭牌：铭牌槽独占、节日款到期删除、卸下后永久款可穿。
package item_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/item/catalog"
	"github.com/Linuxea/item-system/memory"
)

func nameplateStack(t *testing.T) (*item.Inventory, *memory.Stack, *time.Time) {
	t.Helper()
	inv, s, clock := newStack(t)
	s.Defs.Register(catalog.Nameplates()...)
	return inv, s, clock
}

func TestNameplatePermanentAndExpirable(t *testing.T) {
	ctx := context.Background()
	inv, s, clock := nameplateStack(t)

	festival := grantAndEquip(t, inv, "p1", "np_festival_72h")
	perm, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "np_ember", Count: 1})
	if err != nil {
		t.Fatalf("grant perm plate: %v", err)
	}
	if err := inv.Equip(ctx, "p1", perm.InstanceID); !errors.Is(err, item.ErrSlotFull) {
		t.Fatalf("expected ErrSlotFull for exclusive nameplate, got %v", err)
	}

	snap, _ := inv.Build(ctx, "p1", "profile_home")
	if len(snap.Slots[item.SlotNamePlate]) != 1 || snap.Slots[item.SlotNamePlate][0].DefID != "np_festival_72h" {
		t.Fatalf("nameplate slot wrong: %+v", snap.Slots[item.SlotNamePlate])
	}

	if err := inv.Unequip(ctx, "p1", festival); err != nil {
		t.Fatalf("unequip festival: %v", err)
	}
	forceExpire(t, s, festival, clock.Add(-time.Second))
	if n, err := inv.RunExpiry(ctx, 10); err != nil || n != 1 {
		t.Fatalf("expiry: n=%d err=%v", n, err)
	}

	if _, err := s.Instances.Get(ctx, festival); err == nil {
		t.Fatalf("expired festival plate should be removed")
	}

	var evicted bool
	for _, e := range s.Bus.Recorded() {
		if ev, ok := e.(item.Expired); ok && ev.InstanceID == festival {
			evicted = true
			if ev.Policy != item.PolicyRemove {
				t.Fatalf("wrong policy: %s", ev.Policy)
			}
		}
	}
	if !evicted {
		t.Fatalf("expected expiry event")
	}

	if err := inv.Equip(ctx, "p1", perm.InstanceID); err != nil {
		t.Fatalf("equip permanent plate: %v", err)
	}
	snap, _ = inv.Build(ctx, "p1", "profile_home")
	if len(snap.Slots[item.SlotNamePlate]) != 1 || snap.Slots[item.SlotNamePlate][0].DefID != "np_ember" {
		t.Fatalf("permanent plate should be equippable after removal: %+v", snap.Slots[item.SlotNamePlate])
	}
}
