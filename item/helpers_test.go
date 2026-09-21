// helpers_test.go 测试公共设施：带固定时钟的内存栈、强制过期、
// 固定随机源与"发放并穿戴"快捷方式。
// 测试用外部包 item_test：经 memory/item/catalog 间接引用被测包，避免循环导入。
package item_test

import (
	"context"
	"testing"
	"time"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/memory"
)

// fixedRand 固定随机源：next 恒返回 n（宝箱测试注入）。
type fixedRand struct{ n int64 }

func (f *fixedRand) next(int64) int64 { return f.n }

// newStack 装配带固定时钟的内存栈；道具定义由各测试按需注册
// （定义常持有 Levels/Banner 等组件，先建栈再注册是常规顺序）。
func newStack(t *testing.T) (*item.Inventory, *memory.Stack, *time.Time) {
	t.Helper()
	now := time.Now()
	clock := &now
	s := memory.NewStack()
	s.Inv.Now = func() time.Time { return *clock }
	return s.Inv, s, clock
}

// forceExpire 强制把实例过期时间改写为指定时刻（版本 CAS 更新），
// 免去推进时钟等待。
func forceExpire(t *testing.T, s *memory.Stack, instanceID string, at time.Time) {
	t.Helper()
	ctx := context.Background()
	inst, err := s.Instances.Get(ctx, instanceID)
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	expect := inst.Version
	inst.ExpireAt = &at
	inst.BumpVersion()
	if err := s.Instances.Update(ctx, inst, expect); err != nil {
		t.Fatalf("force expire: %v", err)
	}
}

// grantAndEquip 发放一个单位并立即穿戴，返回实例 ID。
func grantAndEquip(t *testing.T, inv *item.Inventory, owner, defID string) string {
	t.Helper()
	ctx := context.Background()
	r, err := inv.Grant(ctx, item.GrantRequest{Owner: owner, DefID: defID, Count: 1})
	if err != nil {
		t.Fatalf("grant %s: %v", defID, err)
	}
	if err := inv.Equip(ctx, owner, r.InstanceID); err != nil {
		t.Fatalf("equip %s: %v", defID, err)
	}
	return r.InstanceID
}
