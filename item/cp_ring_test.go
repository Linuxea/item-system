// cp_ring_test.go CP 戒指：BindCP 成对发放并自动穿戴、无关系不可穿、
// 解除联动卸下双方、戒指时效与关系独立。
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

func cpRingStack(t *testing.T) (*item.Inventory, *memory.Stack, *time.Time) {
	t.Helper()
	inv, s, clock := newStack(t)
	s.Defs.Register(catalog.CPRings()...)
	return inv, s, clock
}

func TestBindCPGrantsAndEquipsRingsForBoth(t *testing.T) {
	ctx := context.Background()
	inv, s, _ := cpRingStack(t)

	rel, err := inv.BindCP(ctx, "alice", "bob", "ring_cp_diamond")
	if err != nil {
		t.Fatalf("bind cp: %v", err)
	}

	for _, owner := range []string{"alice", "bob"} {
		instances, _ := s.Instances.ListByOwner(ctx, owner)
		if len(instances) != 1 {
			t.Fatalf("%s should own exactly 1 ring, got %d", owner, len(instances))
		}
		inst := instances[0]
		if inst.DefID != "ring_cp_diamond" || inst.Status != item.StatusEquipped || !inst.Bound {
			t.Fatalf("%s ring state wrong: %+v", owner, inst)
		}

		snap, _ := inv.Build(ctx, owner, "profile_home")
		rings := snap.Slots[item.SlotCPRing]
		if len(rings) != 1 || rings[0].DefID != "ring_cp_diamond" {
			t.Fatalf("%s cp ring slot wrong: %+v", owner, rings)
		}
		found := false
		for _, m := range snap.Modifiers {
			if m.Key == "cp_badge" && m.Value == "diamond" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s cp modifier missing: %+v", owner, snap.Modifiers)
		}
	}

	var boundSeen bool
	for _, e := range s.Bus.Recorded() {
		if ev, ok := e.(relation.Bound); ok && ev.RelationID == rel.ID && ev.PartyA == "alice" && ev.PartyB == "bob" {
			boundSeen = true
		}
	}
	if !boundSeen {
		t.Fatalf("expected relation bound event")
	}
}

func TestCPRingRequiresCPAndDissolveUnequipsBoth(t *testing.T) {
	ctx := context.Background()
	inv, s, _ := cpRingStack(t)

	r, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "ring_cp_gold", Count: 1})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := inv.Equip(ctx, "p1", r.InstanceID); !errors.Is(err, item.ErrConditionNotMet) {
		t.Fatalf("expected ErrConditionNotMet without cp, got %v", err)
	}

	rel, err := inv.BindCP(ctx, "p1", "p2", "ring_cp_gold")
	if err != nil {
		t.Fatalf("bind cp: %v", err)
	}
	preGranted, err := s.Instances.Get(ctx, r.InstanceID)
	if err != nil {
		t.Fatalf("get pre-granted ring: %v", err)
	}
	if preGranted.Status != item.StatusNormal {
		t.Fatalf("pre-granted ring should stay unequipped: %+v", preGranted)
	}
	snap, _ := inv.Build(ctx, "p1", "profile_home")
	if len(snap.Slots[item.SlotCPRing]) != 1 {
		t.Fatalf("p1 should wear the bindcp ring: %+v", snap.Slots[item.SlotCPRing])
	}

	if err := inv.DissolveCP(ctx, rel.ID); err != nil {
		t.Fatalf("dissolve: %v", err)
	}

	for _, owner := range []string{"p1", "p2"} {
		instances, _ := s.Instances.ListByOwner(ctx, owner)
		for _, inst := range instances {
			if inst.DefID != "ring_cp_gold" {
				continue
			}
			if inst.Status != item.StatusNormal {
				t.Fatalf("%s ring should stay owned but unequipped: %+v", owner, inst)
			}
		}
		snap, _ := inv.Build(ctx, owner, "profile_home")
		if len(snap.Slots[item.SlotCPRing]) != 0 {
			t.Fatalf("%s cp ring slot should be empty", owner)
		}
	}

	if _, err := inv.BindCP(ctx, "p1", "p3", "ring_cp_gold"); err != nil {
		t.Fatalf("rebind after dissolve should succeed: %v", err)
	}
}

func TestCPRingExpiryIndependentOfRelation(t *testing.T) {
	ctx := context.Background()
	inv, s, clock := cpRingStack(t)

	rel, err := inv.BindCP(ctx, "p1", "p2", "ring_cp_gold")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	ringID := ""
	instances, _ := s.Instances.ListByOwner(ctx, "p1")
	for _, inst := range instances {
		ringID = inst.ID
	}
	forceExpire(t, s, ringID, clock.Add(-time.Second))
	if n, err := inv.RunExpiry(ctx, 10); err != nil || n != 1 {
		t.Fatalf("ring expiry: n=%d err=%v", n, err)
	}
	if _, err := s.Instances.Get(ctx, ringID); err == nil {
		t.Fatalf("expired ring should be removed")
	}

	if _, err := s.Relations.Get(ctx, rel.ID); err != nil {
		t.Fatalf("relation must survive ring expiry: %v", err)
	}
	snap, _ := inv.Build(ctx, "p2", "profile_home")
	if len(snap.Slots[item.SlotCPRing]) != 1 {
		t.Fatalf("partner ring should remain equipped: %+v", snap.Slots[item.SlotCPRing])
	}

	var expiredEvt bool
	for _, e := range s.Bus.Recorded() {
		if ev, ok := e.(item.Expired); ok && ev.InstanceID == ringID {
			expiredEvt = true
		}
	}
	if !expiredEvt {
		t.Fatalf("expected item expiry event for ring")
	}
}
