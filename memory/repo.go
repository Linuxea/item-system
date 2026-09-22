// Package memory 提供全部端口的内存实现：仓储、幂等、事件总线，
// 以及几个用于演示与测试的外部服务替身（用户服务、账本、广播、等级）。
//
// 它同时是接真实存储时的对照实现：InstanceRepo 的条件更新对应 MySQL 的
// UPDATE ... WHERE version = ?，ListExpiredBefore 对应按到期时间的索引扫描
// 或 Redis 的到期 ZSet。
package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Linuxea/item-system/event"
	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/relation"
)

// InstanceRepo 实例仓储的内存实现。
// 所有读写都返回副本，避免调用方持有内部指针后绕过版本校验直接改数据。
type InstanceRepo struct {
	mu   sync.RWMutex
	data map[string]*item.Instance
}

// NewInstanceRepo 创建空仓储。
func NewInstanceRepo() *InstanceRepo {
	return &InstanceRepo{data: map[string]*item.Instance{}}
}

// Get 按 ID 取实例。
func (r *InstanceRepo) Get(_ context.Context, id string) (*item.Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	inst, ok := r.data[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", item.ErrNoSuchInstance, id)
	}
	return inst.Clone(), nil
}

// ListByOwner 列出玩家的全部实例，按获得时间升序（保证遍历顺序稳定）。
func (r *InstanceRepo) ListByOwner(_ context.Context, owner string) ([]*item.Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*item.Instance
	for _, inst := range r.data {
		if inst.Owner == owner {
			out = append(out, inst.Clone())
		}
	}
	sortInstances(out)
	return out, nil
}

// ListExpiredBefore 列出在给定时刻之前已到期、且尚未被处置过的实例。
func (r *InstanceRepo) ListExpiredBefore(_ context.Context, t time.Time) ([]*item.Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*item.Instance
	for _, inst := range r.data {
		if inst.ExpiryHandled || !inst.Expired(t) {
			continue
		}
		out = append(out, inst.Clone())
	}
	sortInstances(out)
	return out, nil
}

// Create 新建实例。
func (r *InstanceRepo) Create(_ context.Context, inst *item.Instance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.data[inst.ID]; exists {
		return fmt.Errorf("memory: 实例 %s 已存在", inst.ID)
	}
	r.data[inst.ID] = inst.Clone()
	return nil
}

// Update 条件更新：仅当存储中的版本等于 expected 时写入，否则返回 ErrVersionConflict。
// 这一条就是乐观锁 —— 接 MySQL 时对应 UPDATE ... WHERE id = ? AND version = ?，
// 影响行数为 0 即冲突。
func (r *InstanceRepo) Update(_ context.Context, inst *item.Instance, expected int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.data[inst.ID]
	if !ok {
		return fmt.Errorf("%w: %s", item.ErrNoSuchInstance, inst.ID)
	}
	if cur.Version != expected {
		return fmt.Errorf("%w: %s 期望版本 %d，实际 %d", item.ErrVersionConflict, inst.ID, expected, cur.Version)
	}
	r.data[inst.ID] = inst.Clone()
	return nil
}

// Delete 条件删除：版本不匹配时返回 ErrVersionConflict。
func (r *InstanceRepo) Delete(_ context.Context, id string, expected int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.data[id]
	if !ok {
		return fmt.Errorf("%w: %s", item.ErrNoSuchInstance, id)
	}
	if cur.Version != expected {
		return fmt.Errorf("%w: %s 期望版本 %d，实际 %d", item.ErrVersionConflict, id, expected, cur.Version)
	}
	delete(r.data, id)
	return nil
}

// sortInstances 按获得时间、再按 ID 排序，让遍历顺序可预期。
func sortInstances(list []*item.Instance) {
	sort.SliceStable(list, func(i, j int) bool {
		if !list[i].AcquiredAt.Equal(list[j].AcquiredAt) {
			return list[i].AcquiredAt.Before(list[j].AcquiredAt)
		}
		return list[i].ID < list[j].ID
	})
}

// Idempotency 幂等键存储的内存实现。
type Idempotency struct {
	mu   sync.Mutex
	seen map[string]bool
}

// NewIdempotency 创建空的幂等键存储。
func NewIdempotency() *Idempotency { return &Idempotency{seen: map[string]bool{}} }

// Claim 抢占幂等键；首次返回 true，重复返回 false。
func (s *Idempotency) Claim(_ context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[key] {
		return false, nil
	}
	s.seen[key] = true
	return true, nil
}

// Release 释放幂等键，供业务失败后重试。
func (s *Idempotency) Release(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.seen, key)
	return nil
}

// EventBus 事件总线的内存实现：记录全部事件，并支持订阅。
type EventBus struct {
	mu     sync.Mutex
	events []event.Event
	subs   []func(event.Event)
}

// NewEventBus 创建事件总线。
func NewEventBus() *EventBus { return &EventBus{} }

// Publish 记录并分发事件。
func (b *EventBus) Publish(_ context.Context, e event.Event) {
	b.mu.Lock()
	b.events = append(b.events, e)
	subs := append([]func(event.Event){}, b.subs...)
	b.mu.Unlock()
	for _, fn := range subs {
		fn(e)
	}
}

// Subscribe 注册一个订阅者。
func (b *EventBus) Subscribe(fn func(event.Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs = append(b.subs, fn)
}

// Events 返回已记录事件的副本。
func (b *EventBus) Events() []event.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]event.Event{}, b.events...)
}

// CountOf 统计某类事件的条数，测试里常用。
func (b *EventBus) CountOf(k event.Kind) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, e := range b.events {
		if e.Kind == k {
			n++
		}
	}
	return n
}

// RelationRepo 关系仓储的内存实现。
type RelationRepo struct {
	mu   sync.RWMutex
	data map[string]*relation.Relation
}

// NewRelationRepo 创建空的关系仓储。
func NewRelationRepo() *RelationRepo {
	return &RelationRepo{data: map[string]*relation.Relation{}}
}

// Save 新建关系。
func (r *RelationRepo) Save(_ context.Context, rel *relation.Relation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := *rel
	r.data[rel.ID] = &c
	return nil
}

// Get 按 ID 取关系。
func (r *RelationRepo) Get(_ context.Context, id string) (*relation.Relation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rel, ok := r.data[id]
	if !ok {
		return nil, fmt.Errorf("memory: 关系 %s 不存在", id)
	}
	c := *rel
	return &c, nil
}

// Update 更新关系。
func (r *RelationRepo) Update(_ context.Context, rel *relation.Relation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.data[rel.ID]; !ok {
		return fmt.Errorf("memory: 关系 %s 不存在", rel.ID)
	}
	c := *rel
	r.data[rel.ID] = &c
	return nil
}

// ListByOwner 列出与某玩家相关的全部关系。
func (r *RelationRepo) ListByOwner(_ context.Context, owner string) ([]*relation.Relation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*relation.Relation
	for _, rel := range r.data {
		if rel.Involves(owner) {
			c := *rel
			out = append(out, &c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ListAll 列出全部关系。
func (r *RelationRepo) ListAll(_ context.Context) ([]*relation.Relation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*relation.Relation
	for _, rel := range r.data {
		c := *rel
		out = append(out, &c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
