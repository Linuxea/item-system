package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/linuxea/item-system/catalog"
	"github.com/linuxea/item-system/domain/event"
	"github.com/linuxea/item-system/domain/grant"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/profile"
	"github.com/linuxea/item-system/domain/relation"
)

func TestBindCPGrantsAndEquipsRingsForBoth(t *testing.T) {
	ctx := context.Background()
	app, s, _ := newStackWithClock(t, catalog.CPRings())

	rel, err := app.BindCP(ctx, "alice", "bob", "ring_cp_diamond")
	if err != nil {
		t.Fatalf("bind cp: %v", err)
	}

	for _, owner := range []string{"alice", "bob"} {
		instances, _ := s.Instances.ListByOwner(ctx, owner)
		if len(instances) != 1 {
			t.Fatalf("%s should own exactly 1 ring, got %d", owner, len(instances))
		}
		inst := instances[0]
		if inst.TemplateID != "ring_cp_diamond" || inst.Status != model.StatusEquipped || !inst.Bound {
			t.Fatalf("%s ring state wrong: %+v", owner, inst)
		}

		snap, _ := app.BuildProfile(ctx, owner, "profile_home")
		rings := snap.Slots[model.SlotCPRing]
		if len(rings) != 1 || rings[0].TemplateID != "ring_cp_diamond" {
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
	app, s, _ := newStackWithClock(t, catalog.CPRings())

	r, err := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "ring_cp_gold", Count: 1})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := app.Equip(ctx, "p1", r.InstanceID); err != profile.ErrConditionNotMet {
		t.Fatalf("expected ErrConditionNotMet without cp, got %v", err)
	}

	rel, err := app.BindCP(ctx, "p1", "p2", "ring_cp_gold")
	if err != nil {
		t.Fatalf("bind cp: %v", err)
	}
	preGranted, err := s.Instances.Get(ctx, r.InstanceID)
	if err != nil {
		t.Fatalf("get pre-granted ring: %v", err)
	}
	if preGranted.Status != model.StatusNormal {
		t.Fatalf("pre-granted ring should stay unequipped: %+v", preGranted)
	}
	snap, _ := app.BuildProfile(ctx, "p1", "profile_home")
	if len(snap.Slots[model.SlotCPRing]) != 1 {
		t.Fatalf("p1 should wear the bindcp ring: %+v", snap.Slots[model.SlotCPRing])
	}

	if err := app.DissolveCP(ctx, rel.ID); err != nil {
		t.Fatalf("dissolve: %v", err)
	}

	for _, owner := range []string{"p1", "p2"} {
		instances, _ := s.Instances.ListByOwner(ctx, owner)
		for _, inst := range instances {
			if inst.TemplateID != "ring_cp_gold" {
				continue
			}
			if inst.Status != model.StatusNormal {
				t.Fatalf("%s ring should stay owned but unequipped: %+v", owner, inst)
			}
		}
		snap, _ := app.BuildProfile(ctx, owner, "profile_home")
		if len(snap.Slots[model.SlotCPRing]) != 0 {
			t.Fatalf("%s cp ring slot should be empty", owner)
		}
	}

	if _, err := app.BindCP(ctx, "p1", "p3", "ring_cp_gold"); err != nil {
		t.Fatalf("rebind after dissolve should succeed: %v", err)
	}
}

func TestCPRingExpiryIndependentOfRelation(t *testing.T) {
	ctx := context.Background()
	app, s, clock := newStackWithClock(t, catalog.CPRings())

	rel, err := app.BindCP(ctx, "p1", "p2", "ring_cp_gold")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	ringID := ""
	instances, _ := s.Instances.ListByOwner(ctx, "p1")
	for _, inst := range instances {
		ringID = inst.ID
	}
	forceExpire(t, s, ringID, clock.Add(-time.Second))
	if n, err := app.RunExpiry(ctx, 10); err != nil || n != 1 {
		t.Fatalf("ring expiry: n=%d err=%v", n, err)
	}
	if _, err := s.Instances.Get(ctx, ringID); err == nil {
		t.Fatalf("expired ring should be removed")
	}

	if _, err := s.Relations.Get(ctx, rel.ID); err != nil {
		t.Fatalf("relation must survive ring expiry: %v", err)
	}
	snap, _ := app.BuildProfile(ctx, "p2", "profile_home")
	if len(snap.Slots[model.SlotCPRing]) != 1 {
		t.Fatalf("partner ring should remain equipped: %+v", snap.Slots[model.SlotCPRing])
	}

	var expiredEvt bool
	for _, e := range s.Bus.Recorded() {
		if ev, ok := e.(event.Expired); ok && ev.InstanceID == ringID {
			expiredEvt = true
		}
	}
	if !expiredEvt {
		t.Fatalf("expected item expiry event for ring")
	}
}
