// rename_card_test.go 改名卡：使用调用用户服务改名、扣减由通用层完成、
// 缺参数零消耗零改名且幂等键释放。这是"道具行为写在类型方法里"的最直接示范。
package item_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/item/catalog"
	"github.com/Linuxea/item-system/memory"
)

func renameStack(t *testing.T) (*item.Inventory, *memory.Stack) {
	t.Helper()
	inv, s, _ := newStack(t)
	s.Defs.Register(catalog.RenameCards(s.Renamer)...)
	return inv, s
}

func TestRenameCardUse(t *testing.T) {
	ctx := context.Background()
	inv, s := renameStack(t)

	r, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "rename_card", Count: 2})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}

	res, err := inv.Use(ctx, item.UseRequest{
		Owner:      "p1",
		InstanceID: r.InstanceID,
		Params:     map[string]any{"new_name": "新名字"},
	})
	if err != nil {
		t.Fatalf("use: %v", err)
	}
	if res.Consumed != 1 {
		t.Fatalf("expected 1 consumed, got %d", res.Consumed)
	}

	if got := s.Renamer.Current["p1"]; got != "新名字" {
		t.Fatalf("expected rename to 新名字, got %q", got)
	}
	records := s.Renamer.Records()
	if len(records) != 1 || records[0].Owner != "p1" || records[0].NewName != "新名字" {
		t.Fatalf("rename record wrong: %+v", records)
	}

	inst, _ := s.Instances.Get(ctx, r.InstanceID)
	if inst.Count != 1 {
		t.Fatalf("expected 1 card left, got %d", inst.Count)
	}
}

func TestRenameCardWithoutNameFails(t *testing.T) {
	ctx := context.Background()
	inv, s := renameStack(t)

	r, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "rename_card", Count: 1})

	if _, err := inv.Use(ctx, item.UseRequest{
		Owner:          "p1",
		InstanceID:     r.InstanceID,
		IdempotencyKey: "op-1",
		Params:         map[string]any{"new_name": ""},
	}); !errors.Is(err, item.ErrMissingParam) {
		t.Fatalf("expected ErrMissingParam, got %v", err)
	}

	inst, _ := s.Instances.Get(ctx, r.InstanceID)
	if inst.Count != 1 {
		t.Fatalf("failed use must not consume, got %d", inst.Count)
	}
	if len(s.Renamer.Records()) != 0 {
		t.Fatalf("no rename expected on failure")
	}

	if _, err := inv.Use(ctx, item.UseRequest{
		Owner:          "p1",
		InstanceID:     r.InstanceID,
		IdempotencyKey: "op-1",
		Params:         map[string]any{"new_name": "另一个名字"},
	}); err != nil {
		t.Fatalf("idempotency key should be released after failure: %v", err)
	}
	if _, err := s.Instances.Get(ctx, r.InstanceID); err == nil {
		t.Fatalf("card should be fully consumed and removed")
	}
	if got := s.Renamer.Current["p1"]; got != "另一个名字" {
		t.Fatalf("expected rename to 另一个名字, got %q", got)
	}
}
