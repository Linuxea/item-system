// def.go 道具定义的核心词汇：Def 基础接口、Common 公共数据、可选能力接口。
// 每种道具是一个显式 Go 类型：内嵌 Common 获得展示标签，按需实现下面的
// 可选能力接口；通用层（Inventory）通过类型断言发现能力，不做任何配置解码。
package item

import (
	"context"
	"time"

	"github.com/Linuxea/item-system/relation"
)

// Def 道具定义的基础接口：标识与展示标签。每种道具一个 Go 类型，
// 内嵌 Common 即可自动满足本接口。
type Def interface {
	// DefID 道具定义唯一标识，实例通过 Instance.DefID 关联。
	DefID() string
	// DisplayName 展示名称。
	DisplayName() string
	// SortPriority 展示排序优先级，越大越靠前。
	SortPriority() int
	// SortRarity 稀有度，参与展示排序。
	SortRarity() int
	// StackLimit 单实例数量上限；小于等于 1 视为不可堆叠。
	StackLimit() int64
	// BindOnPickup 是否拾取即绑定（发放时置位 Bound，不可交易）。
	BindOnPickup() bool
}

// Common 道具定义的公共数据部分。道具类型内嵌它获得 Def 接口的默认实现，
// 零值语义：StackLimit 为 0/1 即不可堆叠，BindOnPickup 为 false 即不绑定。
type Common struct {
	// ID 道具定义唯一标识。
	ID string
	// Name 展示名称。
	Name string
	// Category 品类标签（仅展示）。
	Category Category
	// Priority 展示排序优先级，越大越靠前。
	Priority int
	// Rarity 稀有度。
	Rarity int
	// MaxStack 单实例数量上限，0/1 视为不可堆叠。
	MaxStack int64
	// Bind 拾取即绑定。
	Bind bool
}

// DefID 返回道具定义标识。
func (c Common) DefID() string { return c.ID }

// DisplayName 返回展示名称。
func (c Common) DisplayName() string { return c.Name }

// SortPriority 返回展示排序优先级。
func (c Common) SortPriority() int { return c.Priority }

// SortRarity 返回稀有度。
func (c Common) SortRarity() int { return c.Rarity }

// StackLimit 返回单实例数量上限。
func (c Common) StackLimit() int64 { return c.MaxStack }

// BindOnPickup 报告是否拾取即绑定。
func (c Common) BindOnPickup() bool { return c.Bind }

// Usable 可主动使用的道具实现：Use 里写这个道具的全部业务逻辑
// （改名卡调用用户服务、飘屏卡调用广播端口、宝箱调用 inv.Grant 再发放……）。
// 约定：需要用户参数的道具应在产生任何副作用之前校验 params 并返回
// ErrMissingParam，保证"缺参数零消耗"。
type Usable interface {
	// Use 对一个单位执行使用逻辑；通用层负责校验归属/数量并在成功后扣减。
	// inv 提供发放等通用能力，外部业务服务由道具类型自己持有。
	Use(ctx context.Context, inv *Inventory, owner string, params map[string]any) error
}

// Equippable 可穿戴的道具实现：声明目标槽位与该槽位对此道具的容量。
type Equippable interface {
	// EquipSlot 目标穿戴槽位。
	EquipSlot() Slot
	// Capacity 该槽位可同时穿戴的此道具数量上限。
	Capacity() int
}

// EquipGated 有穿戴前置条件的道具实现：条件由道具自己判断
// （查等级源、查关系仓储等），通用层只问结果不代劳。
type EquipGated interface {
	// CanEquip 判断玩家是否满足穿戴条件。
	CanEquip(ctx context.Context, inv *Inventory, owner string) (bool, error)
}

// RelationGated 穿戴依赖生效关系的道具实现（关系卡、CP 戒指等）。
// 通用层在关系解除/到期时据此联动卸下相关道具。
type RelationGated interface {
	// RequiresRelation 穿戴所依赖的关系类型。
	RequiresRelation() relation.Type
}

// Expirable 有时效的道具实现：声明有效期与过期策略。
// 策略由通用层统一执行；降级目标继承目标道具自身的时效（支持链式降级）。
type Expirable interface {
	// Lifetime 发放时的有效期；小于等于 0 视为永久。
	Lifetime() time.Duration
	// ExpirePolicy 过期策略：PolicyRemove / PolicyUnequip / PolicyDowngrade。
	ExpirePolicy() string
	// DowngradeTo 降级目标的道具定义标识，仅 PolicyDowngrade 时生效。
	DowngradeTo() string
}

// Passive 穿戴后提供被动修饰的道具实现：修饰符由快照聚合。
type Passive interface {
	// Modifiers 返回该道具的被动修饰符集合。
	Modifiers() []Modifier
}
