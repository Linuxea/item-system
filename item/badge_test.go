// badge_test.go 勋章：多容量槽并穿、超容量拒绝、卸下腾位、快照排序。
package item_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/item/catalog"
	"github.com/Linuxea/item-system/memory"
)

func badgeStack(t *testing.T) (*item.Inventory, *memory.Stack) {
	t.Helper()
	inv, s, _ := newStack(t)
	s.Defs.Register(catalog.Badges()...)
	return inv, s
}

func TestBadgeSlotCapacityAndOrder(t *testing.T) {
	ctx := context.Background()
	inv, _ := badgeStack(t)

	grantAndEquip(t, inv, "p1", "badge_rookie")
	grantAndEquip(t, inv, "p1", "badge_flame")
	grantAndEquip(t, inv, "p1", "badge_abyss")

	abyss, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "badge_abyss", Count: 1})
	if err != nil {
		t.Fatalf("grant duplicate: %v", err)
	}
	if err := inv.Equip(ctx, "p1", abyss.InstanceID); !errors.Is(err, item.ErrSlotFull) {
		t.Fatalf("expected ErrSlotFull (3 badges already worn), got %v", err)
	}

	rookie2, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "badge_rookie", Count: 1})
	if err := inv.Equip(ctx, "p1", rookie2.InstanceID); !errors.Is(err, item.ErrSlotFull) {
		t.Fatalf("expected ErrSlotFull on 4th badge, got %v", err)
	}

	snap, err := inv.Build(ctx, "p1", "profile_home")
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	badges := snap.Slots[item.SlotBadge]
	if len(badges) != 3 {
		t.Fatalf("expected 3 badges, got %d", len(badges))
	}
	want := []string{"badge_abyss", "badge_flame", "badge_rookie"}
	for i, w := range want {
		if badges[i].DefID != w {
			t.Fatalf("pos %d: want %s got %s", i, w, badges[i].DefID)
		}
	}
}

func TestBadgeUnequipFreesHole(t *testing.T) {
	ctx := context.Background()
	inv, _ := badgeStack(t)

	id := grantAndEquip(t, inv, "p1", "badge_rookie")
	grantAndEquip(t, inv, "p1", "badge_flame")

	if err := inv.Unequip(ctx, "p1", id); err != nil {
		t.Fatalf("unequip: %v", err)
	}
	grantAndEquip(t, inv, "p1", "badge_abyss")

	snap, _ := inv.Build(ctx, "p1", "profile_home")
	if got := len(snap.Slots[item.SlotBadge]); got != 2 {
		t.Fatalf("expected 2 badges after swap, got %d", got)
	}
}
