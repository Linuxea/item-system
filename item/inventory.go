// inventory.go 通用库存服务：与道具种类无关的发放与使用流程。
// 道具行为（如何改名、广播什么）在道具类型的 Use 方法里；
// 这里只负责幂等、校验、堆叠、扣减与事件。
package item

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/Linuxea/item-system/event"
	"github.com/Linuxea/item-system/relation"
)

// 库存层错误。
var (
	// ErrInvalidCount 发放/使用数量必须为正。
	ErrInvalidCount = errors.New("count must be positive")
	// ErrIdempotency 幂等键重复（疑似重复请求）。
	ErrIdempotency = errors.New("duplicate idempotency key")
	// ErrNotOwner 实例不属于该玩家。
	ErrNotOwner = errors.New("instance does not belong to owner")
	// ErrNotUsable 道具未实现 Usable。
	ErrNotUsable = errors.New("item is not usable")
	// ErrNotAvailable 实例当前不可用（已穿戴或已过期）。
	ErrNotAvailable = errors.New("item is not available")
	// ErrInsufficient 实例数量不足。
	ErrInsufficient = errors.New("insufficient item count")
	// ErrMissingParam 使用时缺失必需的用户参数（由道具的 Use 返回）。
	ErrMissingParam = errors.New("required use param missing")
	// ErrNotConfigured 依赖的端口未装配。
	ErrNotConfigured = errors.New("dependency not configured")
)

// DefSource 道具定义源：defID -> Def。定义是只读的代码构造对象，无写入端口。
type DefSource interface {
	// Get 按标识取道具定义。
	Get(ctx context.Context, defID string) (Def, error)
}

// InstanceRepo 实例仓储接口。Update 采用乐观锁：
// expectVersion 必须等于读取时的版本号，否则返回 ErrVersionConflict。
type InstanceRepo interface {
	// Save 新建实例；ID 已存在视为冲突。
	Save(ctx context.Context, inst *Instance) error
	// Get 按 ID 取实例。
	Get(ctx context.Context, id string) (*Instance, error)
	// ListByOwner 列出玩家全部实例（按获取时间升序，保证堆叠合并顺序确定）。
	ListByOwner(ctx context.Context, owner string) ([]*Instance, error)
	// Update 以乐观锁条件更新实例（CAS）。
	Update(ctx context.Context, inst *Instance, expectVersion int64) error
	// Delete 删除实例（幂等）。
	Delete(ctx context.Context, id string) error
	// ListExpired 取 before 时刻前到期的一批实例，limit 控制批量（<=0 不限）。
	ListExpired(ctx context.Context, before time.Time, limit int) ([]*Instance, error)
}

// EquipRepo 穿戴记录仓储接口。
type EquipRepo interface {
	// ListByOwner 列出玩家全部穿戴记录。
	ListByOwner(ctx context.Context, owner string) ([]EquipRecord, error)
	// ListBySlot 列出玩家在指定槽位的穿戴记录（容量检查用）。
	ListBySlot(ctx context.Context, owner string, slot Slot) ([]EquipRecord, error)
	// Save 保存穿戴记录（同键覆盖）。
	Save(ctx context.Context, rec EquipRecord) error
	// Delete 删除指定键的穿戴记录（幂等）。
	Delete(ctx context.Context, owner string, slot Slot, instanceID string) error
	// DeleteByInstance 删除某实例的全部穿戴记录（跨槽位清理）。
	DeleteByInstance(ctx context.Context, instanceID string) error
}

// IdempotencyStore 幂等键存储：Claim 抢占键（false 表示已被占用），
// Release 在业务失败回滚时释放键。
type IdempotencyStore interface {
	Claim(ctx context.Context, key string) (bool, error)
	Release(ctx context.Context, key string) error
}

// ErrNotFound 实体不存在错误，Entity 描述缺失的实体。
type ErrNotFound struct{ Entity string }

// Error 实现错误接口。
func (e *ErrNotFound) Error() string { return e.Entity + " not found" }

// ErrConflict 实体已存在（唯一性冲突）错误。
type ErrConflict struct{ Entity string }

// Error 实现错误接口。
func (e *ErrConflict) Error() string { return e.Entity + " already exists" }

// ErrVersionConflict 乐观锁版本冲突错误：写入时的期望版本与存储中的当前版本不一致。
type ErrVersionConflict struct{ Entity string }

// Error 实现错误接口。
func (e *ErrVersionConflict) Error() string { return e.Entity + " version conflict" }

// GrantRequest 发放请求。
type GrantRequest struct {
	// Owner 目标玩家。
	Owner string
	// DefID 道具定义标识。
	DefID string
	// Count 发放数量。
	Count int64
	// Source 发放来路（gm/shop/effect 等），进事件留痕。
	Source string
	// Reason 发放原因，进事件留痕。
	Reason string
	// IdempotencyKey 幂等键，非空时防重复发放；业务失败会自动释放。
	IdempotencyKey string
	// ExpireAt 显式过期时间；非空时优先于道具自身的时效配置。
	ExpireAt *time.Time
}

// UseRequest 使用请求。
type UseRequest struct {
	// Owner 使用者，须与实例归属一致。
	Owner string
	// InstanceID 目标实例。
	InstanceID string
	// Count 使用数量，0 视为 1。每消耗一个单位执行一轮道具的 Use。
	Count int64
	// IdempotencyKey 幂等键，非空时防重复使用；业务失败会自动释放。
	IdempotencyKey string
	// Params 用户动态输入（如飘屏文案、新名字），直接传给道具的 Use。
	Params map[string]any
}

// Inventory 通用库存服务。字段导出、构造后可直接替换（测试注入时钟等）；
// Defs/Instances/Equips/Idempotency/Events 为必需依赖，
// Relations 供关系流程与道具穿戴条件查询，无关系玩法时可留空。
type Inventory struct {
	// Defs 道具定义源。
	Defs DefSource
	// Instances 实例仓储。
	Instances InstanceRepo
	// Equips 穿戴记录仓储。
	Equips EquipRepo
	// Idempotency 幂等键存储。
	Idempotency IdempotencyStore
	// Events 事件发布端口。
	Events event.Publisher
	// Relations 关系仓储（关系流程与联动使用），可空。
	Relations relation.Repo
	// Sorts 场景排序注册表，nil 时使用默认注册表（未注册场景回落 DefaultSort）。
	Sorts *SortRegistry
	// NewID 实例 ID 生成器，nil 时使用加密随机默认实现。
	NewID func() string
	// Now 时钟，测试可注入固定时钟；nil 时使用 time.Now。
	Now func() time.Time
}

// New 构造库存服务并补默认值（时钟/ID 生成器）。
func New(base Inventory) *Inventory {
	inv := base
	if inv.Now == nil {
		inv.Now = time.Now
	}
	if inv.NewID == nil {
		inv.NewID = randomID
	}
	if inv.Sorts == nil {
		inv.Sorts = NewSortRegistry()
	}
	return &inv
}

// randomID 默认 ID 生成器：16 字节加密随机数hex。
func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// 加密随机源不可用时退化为时间戳，保证可用性。
		return time.Now().Format("20060102150405.000000000")
	}
	return "inst_" + hex.EncodeToString(b)
}

// Grant 执行一次发放：幂等抢占 -> 取定义 -> 应用（堆叠/新建）-> 发布事件。
// 任何一步失败都释放幂等键，保证失败请求可安全重试。
func (inv *Inventory) Grant(ctx context.Context, req GrantRequest) (*GrantResult, error) {
	if req.Count <= 0 {
		return nil, ErrInvalidCount
	}
	if req.IdempotencyKey != "" {
		ok, err := inv.Idempotency.Claim(ctx, req.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrIdempotency
		}
	}

	def, err := inv.Defs.Get(ctx, req.DefID)
	if err != nil {
		inv.release(ctx, req.IdempotencyKey)
		return nil, err
	}

	primaryID, err := inv.apply(ctx, req, def)
	if err != nil {
		inv.release(ctx, req.IdempotencyKey)
		return nil, err
	}

	inv.Events.Publish(Granted{
		Base:       event.Base{At: inv.Now()},
		Owner:      req.Owner,
		DefID:      req.DefID,
		InstanceID: primaryID,
		Count:      req.Count,
		Source:     req.Source,
		Reason:     req.Reason,
	})
	return &GrantResult{InstanceID: primaryID, Count: req.Count}, nil
}

// apply 落库发放数量：先尝试并入既有堆叠（逐个 CAS 更新），余量再新建实例。
// 返回主实例 ID：优先取被合并的第一个实例，否则取新实例。
func (inv *Inventory) apply(ctx context.Context, req GrantRequest, def Def) (string, error) {
	now := inv.Now()
	remain := req.Count
	var primaryID string

	// 仅当可堆叠（StackLimit > 1）时尝试合并，减少实例数量。
	if def.StackLimit() > 1 {
		existing, err := inv.Instances.ListByOwner(ctx, req.Owner)
		if err != nil {
			return "", err
		}
		for _, inst := range existing {
			if remain == 0 {
				break
			}
			// 同定义、当前可用、且时效窗口一致才可合并
			// （时效不同的堆叠道具分开存放，避免"永久+限时"混在一个实例里）。
			if inst.DefID != req.DefID || !inst.Available(now) || !sameExpiry(inst.ExpireAt, req.ExpireAt) {
				continue
			}
			room := def.StackLimit() - inst.Count
			if room <= 0 {
				continue
			}
			take := min(remain, room)
			// 乐观锁写入：以读取到的版本为期望值，并发冲突会失败。
			expect := inst.Version
			inst.Count += take
			inst.BumpVersion()
			if err := inv.Instances.Update(ctx, inst, expect); err != nil {
				return "", err
			}
			remain -= take
			if primaryID == "" {
				primaryID = inst.ID
			}
		}
	}

	// 余量新建实例；过期时间取请求显式值，否则用道具自身时效。
	if remain > 0 {
		inst := &Instance{
			ID:         inv.NewID(),
			DefID:      req.DefID,
			Owner:      req.Owner,
			Count:      remain,
			Bound:      def.BindOnPickup(),
			Status:     StatusNormal,
			AcquiredAt: now,
			Version:    0,
		}
		switch {
		case req.ExpireAt != nil:
			t := *req.ExpireAt
			inst.ExpireAt = &t
		default:
			if exp, ok := def.(Expirable); ok && exp.Lifetime() > 0 {
				t := now.Add(exp.Lifetime())
				inst.ExpireAt = &t
			}
		}
		if err := inv.Instances.Save(ctx, inst); err != nil {
			return "", err
		}
		if primaryID == "" {
			primaryID = inst.ID
		}
	}
	return primaryID, nil
}

// Use 执行一次使用：幂等抢占 -> 归属/状态/数量校验 -> 断言 Usable
// -> 逐单位调用道具的 Use（业务逻辑在道具类型里）
// -> 全部成功后版本 CAS 扣减 -> 发布事件。
// 道具 Use 失败即中止并释放幂等键，保证不出现"扣了道具没给效果"。
func (inv *Inventory) Use(ctx context.Context, req UseRequest) (*UseResult, error) {
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.IdempotencyKey != "" {
		ok, err := inv.Idempotency.Claim(ctx, req.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrIdempotency
		}
	}

	inst, err := inv.Instances.Get(ctx, req.InstanceID)
	if err != nil {
		inv.release(ctx, req.IdempotencyKey)
		return nil, err
	}
	if inst.Owner != req.Owner {
		inv.release(ctx, req.IdempotencyKey)
		return nil, ErrNotOwner
	}
	if !inst.Available(inv.Now()) {
		inv.release(ctx, req.IdempotencyKey)
		return nil, ErrNotAvailable
	}

	def, err := inv.Defs.Get(ctx, inst.DefID)
	if err != nil {
		inv.release(ctx, req.IdempotencyKey)
		return nil, err
	}
	usable, ok := def.(Usable)
	if !ok {
		inv.release(ctx, req.IdempotencyKey)
		return nil, ErrNotUsable
	}
	if inst.Count < req.Count {
		inv.release(ctx, req.IdempotencyKey)
		return nil, ErrInsufficient
	}

	// 每消耗一个单位执行一轮道具逻辑（宝箱开 3 次即抽 3 次）。
	for i := int64(0); i < req.Count; i++ {
		if err := usable.Use(ctx, inv, req.Owner, req.Params); err != nil {
			inv.release(ctx, req.IdempotencyKey)
			return nil, err
		}
	}

	// 扣减：乐观锁写入；扣到 0 直接删除实例。
	expect := inst.Version
	inst.Count -= req.Count
	inst.BumpVersion()
	if inst.Count == 0 {
		if err := inv.Instances.Delete(ctx, inst.ID); err != nil {
			return nil, err
		}
	} else if err := inv.Instances.Update(ctx, inst, expect); err != nil {
		return nil, err
	}

	inv.Events.Publish(Consumed{
		Base:       event.Base{At: inv.Now()},
		Owner:      req.Owner,
		DefID:      inst.DefID,
		InstanceID: inst.ID,
		Count:      req.Count,
	})
	return &UseResult{InstanceID: inst.ID, Consumed: req.Count}, nil
}

// sameExpiry 比较两个可空过期时间是否指向同一时刻。
func sameExpiry(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

// release 业务失败时释放幂等键，允许调用方重试；释放失败静默忽略（不掩盖原始错误）。
func (inv *Inventory) release(ctx context.Context, key string) {
	if key == "" {
		return
	}
	_ = inv.Idempotency.Release(ctx, key)
}
