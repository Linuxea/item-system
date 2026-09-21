// grant_test.go 发放语义：堆叠合并只在同时效窗口发生、幂等键防重与失败释放。
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

func grantStack(t *testing.T) (*item.Inventory, *memory.Stack, *time.Time) {
	t.Helper()
	inv, s, clock := newStack(t)
	s.Defs.Register(catalog.Banners(s.Banner)...)
	return inv, s, clock
}

func TestGrantStackMergesOnlySameExpiry(t *testing.T) {
	ctx := context.Background()
	inv, s, clock := grantStack(t)

	// 永久（无显式过期）与显式过期的发放分开存放。
	perm, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "banner_rose", Count: 1})
	if err != nil {
		t.Fatalf("grant perm: %v", err)
	}
	expireAt := clock.Add(24 * time.Hour)
	limited, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "banner_rose", Count: 2, ExpireAt: &expireAt})
	if err != nil {
		t.Fatalf("grant limited: %v", err)
	}
	if perm.InstanceID == limited.InstanceID {
		t.Fatalf("perm and limited banners must not share an instance")
	}

	// 相同时效窗口的发放合并进同一实例。
	same, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "banner_rose", Count: 3, ExpireAt: &expireAt})
	if err != nil {
		t.Fatalf("grant same: %v", err)
	}
	if same.InstanceID != limited.InstanceID {
		t.Fatalf("same-expiry banners should stack into one instance")
	}
	inst, _ := s.Instances.Get(ctx, limited.InstanceID)
	if inst.Count != 5 {
		t.Fatalf("expected 5 stacked, got %d", inst.Count)
	}
}

func TestGrantIdempotency(t *testing.T) {
	ctx := context.Background()
	inv, _, _ := grantStack(t)

	r1, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "banner_rose", Count: 1, IdempotencyKey: "op-1"})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if r1.Count != 1 {
		t.Fatalf("expected 1 granted, got %d", r1.Count)
	}
	if _, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "banner_rose", Count: 1, IdempotencyKey: "op-1"}); !errors.Is(err, item.ErrIdempotency) {
		t.Fatalf("expected ErrIdempotency, got %v", err)
	}

	// 业务失败的幂等键会被释放：未知定义失败后，同键重试成功。
	if _, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "nope", Count: 1, IdempotencyKey: "op-2"}); err == nil {
		t.Fatalf("unknown def should fail")
	}
	if _, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "banner_rose", Count: 1, IdempotencyKey: "op-2"}); err != nil {
		t.Fatalf("key should be released after failure: %v", err)
	}
}

func TestGrantInvalidCount(t *testing.T) {
	ctx := context.Background()
	inv, _, _ := grantStack(t)
	if _, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "banner_rose", Count: 0}); !errors.Is(err, item.ErrInvalidCount) {
		t.Fatalf("expected ErrInvalidCount, got %v", err)
	}
}
