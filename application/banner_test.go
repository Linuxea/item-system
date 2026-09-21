package application_test

import (
	"context"
	"testing"

	"github.com/linuxea/item-system/catalog"
	"github.com/linuxea/item-system/domain/effect"
	"github.com/linuxea/item-system/domain/grant"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/usage"
)

func TestBannerUseBroadcastsText(t *testing.T) {
	ctx := context.Background()
	app, s, _ := newStackWithClock(t, catalog.Banners())

	r, err := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "banner_rose", Count: 3})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}

	res, err := app.Use(ctx, usage.Request{
		Owner:      "p1",
		InstanceID: r.InstanceID,
		Params:     map[string]any{"text": "p1 加入了公会！"},
	})
	if err != nil {
		t.Fatalf("use: %v", err)
	}
	if res.Consumed != 1 {
		t.Fatalf("expected 1 consumed, got %d", res.Consumed)
	}

	records := s.Banner.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(records))
	}
	if records[0].Owner != "p1" || records[0].Text != "p1 加入了公会！" || records[0].Duration != 10_000_000_000 {
		t.Fatalf("broadcast record wrong: %+v", records[0])
	}

	inst, _ := s.Instances.Get(ctx, r.InstanceID)
	if inst.Count != 2 {
		t.Fatalf("expected 2 banners left, got %d", inst.Count)
	}
}

func TestBannerUseWithoutTextFails(t *testing.T) {
	ctx := context.Background()
	app, s, _ := newStackWithClock(t, catalog.Banners())

	r, _ := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "banner_rose", Count: 2})

	if _, err := app.Use(ctx, usage.Request{Owner: "p1", InstanceID: r.InstanceID, IdempotencyKey: "op-1"}); err != effect.ErrMissingParam {
		t.Fatalf("expected ErrMissingParam, got %v", err)
	}

	inst, _ := s.Instances.Get(ctx, r.InstanceID)
	if inst.Count != 2 {
		t.Fatalf("failed use must not consume, got %d", inst.Count)
	}
	if len(s.Banner.Records()) != 0 {
		t.Fatalf("no broadcast expected on failure")
	}

	if _, err := app.Use(ctx, usage.Request{
		Owner:          "p1",
		InstanceID:     r.InstanceID,
		IdempotencyKey: "op-1",
		Params:         map[string]any{"text": "hello"},
	}); err != nil {
		t.Fatalf("idempotency key should be released after failure: %v", err)
	}

	inst, _ = s.Instances.Get(ctx, r.InstanceID)
	if inst.Count != 1 {
		t.Fatalf("expected 1 banner left, got %d", inst.Count)
	}
}

func TestBannerNotEquippable(t *testing.T) {
	ctx := context.Background()
	app, _, _ := newStackWithClock(t, catalog.Banners())

	r, _ := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "banner_fire_30s", Count: 1})
	if err := app.Equip(ctx, "p1", r.InstanceID); err == nil {
		t.Fatalf("banner should not be equippable")
	}
	_ = model.CategoryConsumable
}
