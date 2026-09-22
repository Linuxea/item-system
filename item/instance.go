package item

import "time"

// Instance 玩家持有的一份道具：运行时状态。
//
// 道具本身（Item 实现）是全服共享的只读原型，状态一律放在实例里。
// Version 是乐观锁版本号：读取时记下，写回时带上，对不上说明期间被人改过，本次作废重来。
type Instance struct {
	// ID 实例唯一标识。
	ID string
	// ItemID 指向道具原型。
	ItemID string
	// Owner 归属玩家。
	Owner string
	// Count 持有数量，可堆叠道具可大于 1。
	Count int64
	// Bound 是否已绑定，绑定后不可交易。
	Bound bool
	// Equipped 是否正在穿戴。
	Equipped bool
	// EquippedAt 最近一次穿戴时间，参与展示排序（后戴的排前面）。
	EquippedAt time.Time
	// AcquiredAt 获得时间。
	AcquiredAt time.Time
	// ExpireAt 到期时间，nil 表示永不过期。
	ExpireAt *time.Time
	// ExpiryHandled 到期是否已被处置过。
	// unequip 策略下实例会留在背包里，靠这个标记避免被过期扫描反复捡起。
	ExpiryHandled bool
	// Version 乐观锁版本号。
	Version int64
}

// Expired 报告实例在给定时刻是否已过期。
func (i *Instance) Expired(now time.Time) bool {
	return i.ExpireAt != nil && !now.Before(*i.ExpireAt)
}

// Clone 返回实例的副本，供仓储在读写之间隔离内存引用。
func (i *Instance) Clone() *Instance {
	c := *i
	if i.ExpireAt != nil {
		t := *i.ExpireAt
		c.ExpireAt = &t
	}
	return &c
}

// Registry 道具注册表：ID -> 道具原型。
// 在启动装配点一次性构建，之后只读，可并发访问。
type Registry struct {
	items map[string]Item
}

// NewRegistry 用道具册构建注册表；重复 ID 以后者为准。
func NewRegistry(items ...Item) *Registry {
	m := make(map[string]Item, len(items))
	for _, it := range items {
		m[it.ID()] = it
	}
	return &Registry{items: m}
}

// Lookup 按 ID 取道具原型。
func (r *Registry) Lookup(id string) (Item, bool) {
	it, ok := r.items[id]
	return it, ok
}

// All 返回全部已登记道具（顺序不保证），供后台列表等场景使用。
func (r *Registry) All() []Item {
	out := make([]Item, 0, len(r.items))
	for _, it := range r.items {
		out = append(out, it)
	}
	return out
}

// 下面几个小工具把「询问能力」这件事收在一处，避免调用方到处写类型断言。

// PriorityOf 返回道具的展示优先级，未实现 Displayable 时为 0。
func PriorityOf(it Item) int {
	if d, ok := it.(Displayable); ok {
		return d.Priority()
	}
	return 0
}

// RarityOf 返回道具的稀有度，未实现 Displayable 时为 0。
func RarityOf(it Item) int {
	if d, ok := it.(Displayable); ok {
		return d.Rarity()
	}
	return 0
}

// SlotOf 返回道具的穿戴槽位；不可穿戴时 ok 为 false。
func SlotOf(it Item) (Slot, bool) {
	if e, ok := it.(Equippable); ok {
		return e.Slot(), true
	}
	return "", false
}
