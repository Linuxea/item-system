package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/linuxea/item-system/application"
	"github.com/linuxea/item-system/catalog"
	"github.com/linuxea/item-system/domain/grant"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/profile"
	"github.com/linuxea/item-system/domain/relation"
	"github.com/linuxea/item-system/infrastructure/memory"
)

func relStack(t *testing.T) (*application.App, *memory.Stack, *time.Time) {
	t.Helper()
	return newStackWithClock(t, catalog.RelationCards())
}

func TestRelationCardRequiresActiveRelation(t *testing.T) {
	ctx := context.Background()
	app, _, _ := relStack(t)

	r, err := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "card_master", Count: 1})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := app.Equip(ctx, "p1", r.InstanceID); err != profile.ErrConditionNotMet {
		t.Fatalf("expected ErrConditionNotMet, got %v", err)
	}

	if _, err := app.BindRelation(ctx, relation.TypeMaster, "p1", "p2", nil); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := app.Equip(ctx, "p1", r.InstanceID); err != nil {
		t.Fatalf("equip after bind: %v", err)
	}
}

func TestRelationCardDissolveAutoUnequips(t *testing.T) {
	ctx := context.Background()
	app, s, _ := relStack(t)

	rel, err := app.BindRelation(ctx, relation.TypeMaster, "p1", "p2", nil)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	card1, _ := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "card_master", Count: 1})
	card2, _ := app.Grant(ctx, grant.Request{Owner: "p2", TemplateID: "card_master", Count: 1})
	if err := app.Equip(ctx, "p1", card1.InstanceID); err != nil {
		t.Fatalf("equip p1: %v", err)
	}
	if err := app.Equip(ctx, "p2", card2.InstanceID); err != nil {
		t.Fatalf("equip p2: %v", err)
	}

	if _, err := app.BindRelation(ctx, relation.TypeMaster, "p1", "p3", nil); err != relation.ErrAlreadyBound {
		t.Fatalf("expected ErrAlreadyBound, got %v", err)
	}

	if err := app.DissolveRelation(ctx, rel.ID); err != nil {
		t.Fatalf("dissolve: %v", err)
	}
	for _, id := range []string{card1.InstanceID, card2.InstanceID} {
		inst, err := s.Instances.Get(ctx, id)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if inst.Status != model.StatusNormal {
			t.Fatalf("card should be unequipped after dissolve: %+v", inst)
		}
	}
	snap, _ := app.BuildProfile(ctx, "p1", "profile_home")
	if len(snap.Slots[model.SlotRelation]) != 0 {
		t.Fatalf("relation slot should be empty: %+v", snap.Slots[model.SlotRelation])
	}
}

func TestRelationExpiryAutoUnequips(t *testing.T) {
	ctx := context.Background()
	app, s, clock := relStack(t)
	s.Relations.UseClock(func() time.Time { return *clock })

	expireAt := clock.Add(time.Minute)
	rel, err := app.BindRelation(ctx, relation.TypeBestie, "p1", "p2", &expireAt)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	card, _ := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "card_bestie", Count: 1})
	if err := app.Equip(ctx, "p1", card.InstanceID); err != nil {
		t.Fatalf("equip: %v", err)
	}

	*clock = clock.Add(2 * time.Minute)
	if n, err := app.RunRelationExpiry(ctx, 10); err != nil || n != 1 {
		t.Fatalf("relation expiry: n=%d err=%v", n, err)
	}
	inst, _ := s.Instances.Get(ctx, card.InstanceID)
	if inst.Status != model.StatusNormal {
		t.Fatalf("card should be unequipped after relation expiry: %+v", inst)
	}
	if _, err := s.Relations.Get(ctx, rel.ID); err != nil {
		t.Fatalf("relation should survive as dissolved record: %v", err)
	}
}
