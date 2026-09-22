package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Linuxea/item-system/item"
)

// TestCPRingRequiresRelation CP 戒指要求先建立 cp 关系才能穿戴。
// 引擎全程不知道「关系」是什么 —— 是戒指自己去问 RelationChecker 的。
func TestCPRingRequiresRelation(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	ring := grant(t, s, "u1", "ring_eternal", 1)

	if err := s.Engine.Equip(ctx, "u1", ring); !errors.Is(err, item.ErrConditionNotMet) {
		t.Fatalf("没有 cp 关系时应拒绝穿戴，实际: %v", err)
	}

	if _, err := s.Relations.Bind(ctx, "cp", "u1", "u2", 0); err != nil {
		t.Fatalf("建立关系失败: %v", err)
	}
	if err := s.Engine.Equip(ctx, "u1", ring); err != nil {
		t.Fatalf("建立关系后应可穿戴: %v", err)
	}
}

// TestCPRingBoundOnGrant CP 戒指发放即绑定。
func TestCPRingBoundOnGrant(t *testing.T) {
	s := newStack(t)
	id := grant(t, s, "u1", "ring_eternal", 1)
	inst, _ := s.Instances.Get(context.Background(), id)
	if !inst.Bound {
		t.Fatal("CP 戒指应发放即绑定")
	}
}

// TestDissolveUnequipsBothSides 解除关系联动卸下双方的相关道具。
func TestDissolveUnequipsBothSides(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	rel, err := s.Relations.Bind(ctx, "cp", "u1", "u2", 0)
	if err != nil {
		t.Fatalf("建立关系失败: %v", err)
	}
	r1 := grant(t, s, "u1", "ring_eternal", 1)
	r2 := grant(t, s, "u2", "ring_eternal", 1)
	for owner, id := range map[string]string{"u1": r1, "u2": r2} {
		if err := s.Engine.Equip(ctx, owner, id); err != nil {
			t.Fatalf("%s 穿戴失败: %v", owner, err)
		}
	}

	if err := s.Relations.Dissolve(ctx, rel.ID, "breakup"); err != nil {
		t.Fatalf("解除关系失败: %v", err)
	}

	for owner, id := range map[string]string{"u1": r1, "u2": r2} {
		inst, err := s.Instances.Get(ctx, id)
		if err != nil {
			t.Fatalf("%s 的戒指应保留: %v", owner, err)
		}
		if inst.Equipped {
			t.Fatalf("%s 的戒指应已被联动卸下", owner)
		}
	}
}

// TestRelationExpiryUnequips 关系到期与主动解除的处置一致。
//
// 这里刻意用永久款关系卡（card_bestie）配限时关系：
// 若用限时关系卡，到期的会是卡本身，测到的就不是关系那条链路了。
func TestRelationExpiryUnequips(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	if _, err := s.Relations.Bind(ctx, "bestie", "u1", "u2", 30*24*time.Hour); err != nil {
		t.Fatalf("建立关系失败: %v", err)
	}
	card := grant(t, s, "u1", "card_bestie", 1)
	if err := s.Engine.Equip(ctx, "u1", card); err != nil {
		t.Fatalf("穿戴失败: %v", err)
	}

	s.Clock.Advance(31 * 24 * time.Hour)
	if err := s.Relations.RunExpiry(ctx); err != nil {
		t.Fatalf("关系过期扫描失败: %v", err)
	}
	inst, _ := s.Instances.Get(ctx, card)
	if inst.Equipped {
		t.Fatal("关系到期后关系卡应被卸下")
	}

	// 关系已失效，重新穿戴应被拒。
	if err := s.Engine.Equip(ctx, "u1", card); !errors.Is(err, item.ErrConditionNotMet) {
		t.Fatalf("关系失效后不应能重新穿戴，实际: %v", err)
	}
}
