// banner_test.go 飘屏卡：使用广播用户文案、缺参数零消耗且幂等键释放、不可穿戴。
package item_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/item/catalog"
	"github.com/Linuxea/item-system/memory"
)

func bannerStack(t *testing.T) (*item.Inventory, *memory.Stack) {
	t.Helper()
	inv, s, _ := newStack(t)
	s.Defs.Register(catalog.Banners(s.Banner)...)
	return inv, s
}

func TestBannerUseBroadcastsText(t *testing.T) {
	ctx := context.Background()
	inv, s := bannerStack(t)

	r, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "banner_rose", Count: 3})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}

	res, err := inv.Use(ctx, item.UseRequest{
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
	inv, s := bannerStack(t)

	r, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "banner_rose", Count: 2})

	if _, err := inv.Use(ctx, item.UseRequest{Owner: "p1", InstanceID: r.InstanceID, IdempotencyKey: "op-1"}); !errors.Is(err, item.ErrMissingParam) {
		t.Fatalf("expected ErrMissingParam, got %v", err)
	}

	inst, _ := s.Instances.Get(ctx, r.InstanceID)
	if inst.Count != 2 {
		t.Fatalf("failed use must not consume, got %d", inst.Count)
	}
	if len(s.Banner.Records()) != 0 {
		t.Fatalf("no broadcast expected on failure")
	}

	if _, err := inv.Use(ctx, item.UseRequest{
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
	inv, _ := bannerStack(t)

	r, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "banner_fire_30s", Count: 1})
	if err := inv.Equip(ctx, "p1", r.InstanceID); !errors.Is(err, item.ErrNotEquippable) {
		t.Fatalf("banner should not be equippable, got %v", err)
	}
}
