// Package model 提供道具领域的纯数据定义：模板、实例、槽位、修饰符。
// 本包不含业务流程、不依赖任何其他领域包，是所有领域服务的共享词汇层。
package model

import "time"

// Category 道具分类，用于区分模板所属的品类（座驾、勋章、VIP 等）。
type Category string

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

// SlotType 穿戴槽位类型；同槽位的道具共享容量约束，并在展示快照中一起排序。
type SlotType string

const (
	// SlotAvatar 头像槽。
	SlotAvatar SlotType = "avatar"
	// SlotNamePlate 铭牌槽。
	SlotNamePlate SlotType = "nameplate"
	// SlotBadge 勋章槽，容量可大于 1。
	SlotBadge SlotType = "badge"
	// SlotVIP VIP 槽，同一玩家同时只保留一个 VIP 身份。
	SlotVIP SlotType = "vip"
	// SlotMount 座驾槽。
	SlotMount SlotType = "mount"
	// SlotHeadwear 头饰槽。
	SlotHeadwear SlotType = "headwear"
	// SlotRelation 关系卡槽。
	SlotRelation SlotType = "relation"
	// SlotCPRing CP 戒指槽。
	SlotCPRing SlotType = "cp_ring"
	// SlotChatBubble 聊天气泡槽。
	SlotChatBubble SlotType = "chat_bubble"
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

// Modifier 修饰符键值对（如 move_speed=80、vip_level=3），
// 由 Passive 行为携带、profile 快照聚合成玩家的展示/属性层。
type Modifier struct {
	Key   string
	Value any
}

// ItemTemplate 道具模板：道具规则的只读配置。
// 规则进模板、状态进实例——模板在运行期不被修改，是"新道具 = 新数据"的落点：
// Behaviors 是行为组件的原始配置（行为 key -> 组件配置），经 behavior.Registry 编译为可用组件。
type ItemTemplate struct {
	// ID 模板唯一标识，实例通过 TemplateID 关联。
	ID string
	// Category 品类标签，仅作分类，不驱动逻辑。
	Category Category
	// Name 展示名称。
	Name string
	// Priority 展示排序优先级，越大越靠前。
	Priority int
	// Rarity 稀有度，参与展示排序。
	Rarity int
	// Behaviors 行为组件配置，key 为 behavior.Key* 常量。
	Behaviors map[string]map[string]any
	// Config 预留的扩展配置位。
	Config map[string]any
}

// ItemInstance 道具实例：玩家持有的运行时数据。
// Version 为乐观锁版本号（CAS）：每次写入前必须 BumpVersion，
// 仓储 Update 需携带读取时的期望版本，不匹配即冲突失败。
type ItemInstance struct {
	// ID 实例唯一标识。
	ID string
	// TemplateID 所属模板。
	TemplateID string
	// Owner 归属玩家。
	Owner string
	// Count 持有数量（堆叠道具可大于 1）。
	Count int64
	// Bound 是否已绑定（拾取即绑定时在发放时置位）。
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
func (i *ItemInstance) Expired(now time.Time) bool {
	return i.ExpireAt != nil && !now.Before(*i.ExpireAt)
}

// Available 报告实例当前是否可用：须处于正常状态且未过期。
func (i *ItemInstance) Available(now time.Time) bool {
	return i.Status == StatusNormal && !i.Expired(now)
}

// BumpVersion 将乐观锁版本号自增并返回新值；写入仓储前调用。
func (i *ItemInstance) BumpVersion() int64 {
	i.Version++
	return i.Version
}

// EquipRecord 穿戴记录：哪个玩家的哪个槽位穿戴了哪个实例。
type EquipRecord struct {
	// Owner 归属玩家。
	Owner string
	// Slot 穿戴槽位。
	Slot SlotType
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
