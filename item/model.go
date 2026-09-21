// Package item 提供道具系统的通用内核：道具定义接口（Def）、可选能力接口
// （Usable/Equippable/Expirable 等）与库存服务（Inventory）。
//
// 设计核心：每种道具是一个显式的 Go 类型——行为写在类型的方法里
// （改名卡.Use 里直接调用用户服务），展示标签内嵌 item.Common；
// 通用层只做与道具种类无关的事（发放/扣减/穿戴记录/过期扫描/关系联动），
// 通过可选接口的类型断言发现道具支持的能力。
package item

import "time"

// Category 道具品类标签，仅用于展示分类，不驱动逻辑。
type Category string

// 常见品类。道具类型本身就是品类，这里只作展示标签。
const (
	// CategoryAvatar 头像框。
	CategoryAvatar Category = "avatar"
	// CategoryNamePlate 铭牌。
	CategoryNamePlate Category = "nameplate"
	// CategoryBadge 勋章。
	CategoryBadge Category = "badge"
	// CategoryVIP 会员。
	CategoryVIP Category = "vip"
	// CategoryConsumable 通用消耗品（药水、宝箱、飘屏卡等）。
	CategoryConsumable Category = "consumable"
	// CategoryCurrency 货币。
	CategoryCurrency Category = "currency"
	// CategoryMount 座驾。
	CategoryMount Category = "mount"
	// CategoryHeadwear 头饰。
	CategoryHeadwear Category = "headwear"
	// CategoryRelationCard 关系卡。
	CategoryRelationCard Category = "relation_card"
	// CategoryCPRing CP 戒指。
	CategoryCPRing Category = "cp_ring"
	// CategoryChatBubble 聊天气泡。
	CategoryChatBubble Category = "chat_bubble"
)

// Slot 穿戴槽位；同槽位的道具共享容量约束，并在展示快照中一起排序。
type Slot string

const (
	// SlotAvatar 头像槽。
	SlotAvatar Slot = "avatar"
	// SlotNamePlate 铭牌槽。
	SlotNamePlate Slot = "nameplate"
	// SlotBadge 勋章槽，容量可大于 1。
	SlotBadge Slot = "badge"
	// SlotVIP VIP 槽，同一玩家同时只保留一个 VIP 身份。
	SlotVIP Slot = "vip"
	// SlotMount 座驾槽。
	SlotMount Slot = "mount"
	// SlotHeadwear 头饰槽。
	SlotHeadwear Slot = "headwear"
	// SlotRelation 关系卡槽。
	SlotRelation Slot = "relation"
	// SlotCPRing CP 戒指槽。
	SlotCPRing Slot = "cp_ring"
	// SlotChatBubble 聊天气泡槽。
	SlotChatBubble Slot = "chat_bubble"
)

// InstanceStatus 道具实例的运行时状态。
type InstanceStatus string

const (
	// StatusNormal 正常状态，可使用、可穿戴。
	StatusNormal InstanceStatus = "normal"
	// StatusEquipped 已穿戴状态。
	StatusEquipped InstanceStatus = "equipped"
	// StatusExpired 已过期状态（为惰性处理场景预留）。
	StatusExpired InstanceStatus = "expired"
)

// 过期策略常量，由 Expirable.ExpirePolicy 返回。
const (
	// PolicyRemove 过期即删除实例（含穿戴记录）。
	PolicyRemove = "remove"
	// PolicyUnequip 过期仅卸下并保留实例。
	PolicyUnequip = "unequip"
	// PolicyDowngrade 过期降级为 DowngradeTo 指定的道具（继承其时效）。
	PolicyDowngrade = "downgrade"
)

// Modifier 修饰符键值对（如 move_speed=80、vip_level=3），
// 由 Passive 道具携带、快照聚合成玩家的展示/属性层。
type Modifier struct {
	Key   string
	Value any
}

// Instance 道具实例：玩家持有的运行时数据。
// 规则在道具定义（Def），状态在这里；Version 为乐观锁版本号（CAS），
// 每次写入前必须 BumpVersion，仓储 Update 携带读取时的期望版本。
type Instance struct {
	// ID 实例唯一标识。
	ID string
	// DefID 所属道具定义标识。
	DefID string
	// Owner 归属玩家。
	Owner string
	// Count 持有数量（可堆叠道具可大于 1）。
	Count int64
	// Bound 是否已绑定（拾取即绑定的道具在发放时置位）。
	Bound bool
	// Status 实例状态。
	Status InstanceStatus
	// AcquiredAt 获取时间。
	AcquiredAt time.Time
	// ExpireAt 过期时间，nil 表示永不过期。
	ExpireAt *time.Time
	// Version 乐观锁版本号。
	Version int64
}

// Expired 报告实例在给定时刻是否已过期；无过期时间的实例永不过期。
func (i *Instance) Expired(now time.Time) bool {
	return i.ExpireAt != nil && !now.Before(*i.ExpireAt)
}

// Available 报告实例当前是否可用：须处于正常状态且未过期。
func (i *Instance) Available(now time.Time) bool {
	return i.Status == StatusNormal && !i.Expired(now)
}

// BumpVersion 将乐观锁版本号自增并返回新值；写入仓储前调用。
func (i *Instance) BumpVersion() int64 {
	i.Version++
	return i.Version
}

// EquipRecord 穿戴记录：哪个玩家的哪个槽位穿戴了哪个实例。
type EquipRecord struct {
	// Owner 归属玩家。
	Owner string
	// Slot 穿戴槽位。
	Slot Slot
	// InstanceID 被穿戴的实例。
	InstanceID string
	// EquippedAt 穿戴时间，参与展示排序。
	EquippedAt time.Time
}

// GrantResult 发放结果：主实例 ID（堆叠合并时可能是既有实例）与本次发放总数。
type GrantResult struct {
	// InstanceID 主实例 ID。
	InstanceID string
	// Count 发放数量。
	Count int64
}

// UseResult 使用结果：被消费的实例 ID 与消费数量。
type UseResult struct {
	// InstanceID 被消费的实例。
	InstanceID string
	// Consumed 消费数量。
	Consumed int64
}
