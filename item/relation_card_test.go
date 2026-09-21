// relation_card_test.go 关系卡：穿戴前置为生效关系、独占绑定、
// 解除联动卸下、关系到期联动卸下。
package item_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/item/catalog"
	"github.com/Linuxea/item-system/memory"
	"github.com/Linuxea/item-system/relation"
)

func relationStack(t *testing.T) (*item.Inventory, *memory.Stack, *time.Time) {
	t.Helper()
	inv, s, clock := newStack(t)
	s.Defs.Register(catalog.RelationCards()...)
	return inv, s, clock
}

func TestRelationCardRequiresActiveRelation(t *testing.T) {
	ctx := context.Background()
	inv, _, _ := relationStack(t)

	r, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "card_master", Count: 1})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := inv.Equip(ctx, "p1", r.InstanceID); !errors.Is(err, item.ErrConditionNotMet) {
		t.Fatalf("expected ErrConditionNotMet, got %v", err)
	}

	if _, err := inv.BindRelation(ctx, relation.TypeMaster, "p1", "p2", nil); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := inv.Equip(ctx, "p1", r.InstanceID); err != nil {
		t.Fatalf("equip after bind: %v", err)
	}
}

func TestRelationCardDissolveAutoUnequips(t *testing.T) {
	ctx := context.Background()
	inv, s, _ := relationStack(t)

	rel, err := inv.BindRelation(ctx, relation.TypeMaster, "p1", "p2", nil)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	card1, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "card_master", Count: 1})
	card2, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p2", DefID: "card_master", Count: 1})
	if err := inv.Equip(ctx, "p1", card1.InstanceID); err != nil {
		t.Fatalf("equip p1: %v", err)
	}
	if err := inv.Equip(ctx, "p2", card2.InstanceID); err != nil {
		t.Fatalf("equip p2: %v", err)
	}

	if _, err := inv.BindRelation(ctx, relation.TypeMaster, "p1", "p3", nil); !errors.Is(err, relation.ErrAlreadyBound) {
		t.Fatalf("expected ErrAlreadyBound, got %v", err)
	}

	if err := inv.DissolveRelation(ctx, rel.ID); err != nil {
		t.Fatalf("dissolve: %v", err)
	}
	for _, id := range []string{card1.InstanceID, card2.InstanceID} {
		inst, err := s.Instances.Get(ctx, id)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if inst.Status != item.StatusNormal {
			t.Fatalf("card should be unequipped after dissolve: %+v", inst)
		}
	}
	snap, _ := inv.Build(ctx, "p1", "profile_home")
	if len(snap.Slots[item.SlotRelation]) != 0 {
		t.Fatalf("relation slot should be empty: %+v", snap.Slots[item.SlotRelation])
	}
}

func TestRelationExpiryAutoUnequips(t *testing.T) {
	ctx := context.Background()
	inv, s, clock := relationStack(t)
	s.Relations.UseClock(func() time.Time { return *clock })

	expireAt := clock.Add(time.Minute)
	rel, err := inv.BindRelation(ctx, relation.TypeBestie, "p1", "p2", &expireAt)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	card, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "card_bestie", Count: 1})
	if err := inv.Equip(ctx, "p1", card.InstanceID); err != nil {
		t.Fatalf("equip: %v", err)
	}

	*clock = clock.Add(2 * time.Minute)
	if n, err := inv.RunRelationExpiry(ctx, 10); err != nil || n != 1 {
		t.Fatalf("relation expiry: n=%d err=%v", n, err)
	}
	inst, _ := s.Instances.Get(ctx, card.InstanceID)
	if inst.Status != item.StatusNormal {
		t.Fatalf("card should be unequipped after relation expiry: %+v", inst)
	}
	if _, err := s.Relations.Get(ctx, rel.ID); err != nil {
		t.Fatalf("relation should survive as dissolved record: %v", err)
	}
}
