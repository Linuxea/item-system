package memory_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Linuxea/item-system/engine"
	"github.com/Linuxea/item-system/event"
	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/memory"
)

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newStack(t *testing.T) *memory.Stack {
	t.Helper()
	s := memory.NewStack(memory.StackOptions{Now: epoch})
	s.Levels.Set("u1", 50)
	s.Levels.Set("u2", 50)
	return s
}

func grant(t *testing.T, s *memory.Stack, owner, itemID string, n int64) string {
	t.Helper()
	res, err := s.Engine.Grant(context.Background(), engine.GrantRequest{
		Owner: owner, ItemID: itemID, Count: n, Source: "test",
	})
	if err != nil {
		t.Fatalf("发放 %s 失败: %v", itemID, err)
	}
	return res.InstanceID
}

// TestStackMergesSameItem 可堆叠道具合并进同一实例。
func TestStackMergesSameItem(t *testing.T) {
	s := newStack(t)
	a := grant(t, s, "u1", "rename_card", 3)
	b := grant(t, s, "u1", "rename_card", 2)
	if a != b {
		t.Fatalf("两次发放应合并进同一实例，得到 %s 和 %s", a, b)
	}
	inst, _ := s.Instances.Get(context.Background(), a)
	if inst.Count != 5 {
		t.Fatalf("合并后数量应为 5，实际 %d", inst.Count)
	}
}

// TestTimedItemsDoNotStack 有时效的道具不合并 —— 每份有自己的到期时刻。
func TestTimedItemsDoNotStack(t *testing.T) {
	s := newStack(t)
	a := grant(t, s, "u1", "mount_ember_7d", 1)
	s.Clock.Advance(time.Hour)
	b := grant(t, s, "u1", "mount_ember_7d", 1)
	if a == b {
		t.Fatal("限时道具不应合并")
	}
	ia, _ := s.Instances.Get(context.Background(), a)
	ib, _ := s.Instances.Get(context.Background(), b)
	if ia.ExpireAt.Equal(*ib.ExpireAt) {
		t.Fatal("两份限时道具的到期时刻应各自独立")
	}
}

// TestGrantIdempotency 同一个幂等键只发一次。
func TestGrantIdempotency(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	req := engine.GrantRequest{Owner: "u1", ItemID: "rename_card", Count: 1, IdempotencyKey: "order-1"}

	if _, err := s.Engine.Grant(ctx, req); err != nil {
		t.Fatalf("首次发放应成功: %v", err)
	}
	_, err := s.Engine.Grant(ctx, req)
	if !errors.Is(err, item.ErrDuplicateRequest) {
		t.Fatalf("重复请求应被拦截，实际: %v", err)
	}
}

// TestUseRenameCard 改名卡：参数经 Bind 进入道具自己的字段。
func TestUseRenameCard(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	id := grant(t, s, "u1", "rename_card", 2)

	res, err := s.Engine.Use(ctx, engine.UseRequest{
		Owner: "u1", InstanceID: id, Params: []byte(`{"new_name":"林夕"}`),
	})
	if err != nil {
		t.Fatalf("使用失败: %v", err)
	}
	if res.Consumed != 1 || res.Remaining != 1 {
		t.Fatalf("应消耗 1 剩 1，实际消耗 %d 剩 %d", res.Consumed, res.Remaining)
	}
	if got := s.Users.NameOf("u1"); got != "林夕" {
		t.Fatalf("昵称应为 林夕，实际 %q", got)
	}
	if s.Bus.CountOf(event.KindConsumed) != 1 {
		t.Fatal("应发出一条 consumed 事件")
	}
}

// TestUseMissingParamHasNoSideEffect 参数缺失时道具分毫不动。
func TestUseMissingParamHasNoSideEffect(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	id := grant(t, s, "u1", "rename_card", 2)

	_, err := s.Engine.Use(ctx, engine.UseRequest{Owner: "u1", InstanceID: id})
	if !errors.Is(err, item.ErrMissingParam) {
		t.Fatalf("应报缺少参数，实际: %v", err)
	}
	inst, _ := s.Instances.Get(ctx, id)
	if inst.Count != 2 {
		t.Fatalf("数量不该变，实际 %d", inst.Count)
	}
	if s.Users.NameOf("u1") != "" {
		t.Fatal("不该发生改名")
	}
}

// TestUseNotUsableItem 座驾没有 Use 方法，引擎据此拒绝。
func TestUseNotUsableItem(t *testing.T) {
	s := newStack(t)
	id := grant(t, s, "u1", "mount_cloud", 1)
	_, err := s.Engine.Use(context.Background(), engine.UseRequest{Owner: "u1", InstanceID: id})
	if !errors.Is(err, item.ErrNotUsable) {
		t.Fatalf("应报不可使用，实际: %v", err)
	}
}

// TestUseBanner 飘屏卡：文案是道具自己的字段。
func TestUseBanner(t *testing.T) {
	s := newStack(t)
	id := grant(t, s, "u1", "banner_rose", 1)
	if _, err := s.Engine.Use(context.Background(), engine.UseRequest{
		Owner: "u1", InstanceID: id, Params: []byte(`{"text":"今天我请客"}`),
	}); err != nil {
		t.Fatalf("使用失败: %v", err)
	}
	sent := s.Broadcaster.Sent()
	if len(sent) != 1 || sent[0].Text != "今天我请客" {
		t.Fatalf("广播内容不对: %+v", sent)
	}
	// 用完即删。
	if _, err := s.Instances.Get(context.Background(), id); !errors.Is(err, item.ErrNoSuchInstance) {
		t.Fatal("用光后实例应被删除")
	}
}

// TestUseChestGrantsPrize 宝箱通过延迟绑定的发放器发出奖品。
func TestUseChestGrantsPrize(t *testing.T) {
	s := memory.NewStack(memory.StackOptions{
		Now:  epoch,
		Rand: func(int64) int64 { return 0 }, // 固定抽第一档：金币袋
	})
	ctx := context.Background()
	id := grant(t, s, "u1", "chest_starter", 3)

	res, err := s.Engine.Use(ctx, engine.UseRequest{Owner: "u1", InstanceID: id, Count: 3})
	if err != nil {
		t.Fatalf("连开三次失败: %v", err)
	}
	if res.Consumed != 3 {
		t.Fatalf("应消耗 3，实际 %d", res.Consumed)
	}
	owned, _ := s.Instances.ListByOwner(ctx, "u1")
	var gold int64
	for _, inst := range owned {
		if inst.ItemID == "pack_gold_100" {
			gold = inst.Count
		}
	}
	if gold != 3 {
		t.Fatalf("应获得 3 个金币袋，实际 %d", gold)
	}
}

// TestConcurrentUseDeductsExactlyOnce 并发闸门：
// 一张改名卡被两个请求同时使用，只能成功一次。
//
// 这正是「先扣减、后执行」的意义。若顺序反过来，两个请求会同时通过数量校验、
// 各自执行一遍改名，之后才有一个扣减失败 —— 道具被用了两次只扣了一次。
func TestConcurrentUseDeductsExactlyOnce(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	id := grant(t, s, "u1", "rename_card", 1)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var okCount, errCount int

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := s.Engine.Use(ctx, engine.UseRequest{
				Owner: "u1", InstanceID: id, Params: []byte(`{"new_name":"林夕"}`),
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				okCount++
			} else {
				errCount++
			}
		}(i)
	}
	wg.Wait()

	if okCount != 1 {
		t.Fatalf("只应有一个请求成功，实际成功 %d 个", okCount)
	}
	if errCount != 7 {
		t.Fatalf("应有 7 个请求失败，实际 %d", errCount)
	}
}
