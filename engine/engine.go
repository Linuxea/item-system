// Package engine 提供驱动全部道具的通用引擎：发放、使用、穿戴、过期、展示快照。
//
// 引擎不认识任何一种具体道具。它只做两件事：
//  1. 维护实例的生命周期（数量、绑定、穿戴、时效、乐观锁、幂等）；
//  2. 在需要分支时，用类型断言询问道具「你有没有某项能力」。
//
// 因此新增一种道具从不需要改动本包；新增一种*能力*才需要（item 包加接口，本包加一处断言）。
package engine

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Linuxea/item-system/event"
	"github.com/Linuxea/item-system/item"
)

// InstanceStore 实例仓储端口。
// Update / Delete 带期望版本号，实现方必须做条件更新（CAS），版本不匹配返回 ErrVersionConflict。
type InstanceStore interface {
	// Get 按 ID 取实例。
	Get(ctx context.Context, id string) (*item.Instance, error)
	// ListByOwner 列出玩家的全部实例。
	ListByOwner(ctx context.Context, owner string) ([]*item.Instance, error)
	// ListExpiredBefore 列出在给定时刻之前已到期的实例，供过期扫描使用。
	ListExpiredBefore(ctx context.Context, t time.Time) ([]*item.Instance, error)
	// Create 新建实例。
	Create(ctx context.Context, inst *item.Instance) error
	// Update 条件更新：仅当存储中的版本等于 expected 时写入。
	Update(ctx context.Context, inst *item.Instance, expected int64) error
	// Delete 条件删除：仅当存储中的版本等于 expected 时删除。
	Delete(ctx context.Context, id string, expected int64) error
}

// IdempotencyStore 幂等键存储端口。
type IdempotencyStore interface {
	// Claim 抢占幂等键；首次抢占返回 true，重复请求返回 false。
	Claim(ctx context.Context, key string) (bool, error)
	// Release 释放幂等键，供业务失败后重试。
	Release(ctx context.Context, key string) error
}

// Options 引擎构造参数。Registry 与 Store 必填，其余可省略。
type Options struct {
	// Registry 道具注册表。
	Registry *item.Registry
	// Store 实例仓储。
	Store InstanceStore
	// Idempotency 幂等键存储；为 nil 时忽略请求里的幂等键。
	Idempotency IdempotencyStore
	// Publisher 事件发布器；为 nil 时使用空实现。
	Publisher event.Publisher
	// Now 时钟；为 nil 时使用 time.Now。测试可注入可推进的假时钟。
	Now func() time.Time
	// NewID 实例 ID 生成器；为 nil 时使用内置自增实现。
	NewID func() string
	// Scenes 各展示场景的排序策略；未配置的场景使用 DefaultSortPolicy。
	Scenes map[string]SortPolicy
}

// Engine 道具引擎。构造后并发安全（并发安全由仓储的 CAS 保证）。
type Engine struct {
	items  *item.Registry
	store  InstanceStore
	idem   IdempotencyStore
	bus    event.Publisher
	now    func() time.Time
	newID  func() string
	scenes map[string]SortPolicy
}

// New 构造引擎。
func New(opts Options) *Engine {
	e := &Engine{
		items:  opts.Registry,
		store:  opts.Store,
		idem:   opts.Idempotency,
		bus:    opts.Publisher,
		now:    opts.Now,
		newID:  opts.NewID,
		scenes: opts.Scenes,
	}
	if e.bus == nil {
		e.bus = event.NopPublisher{}
	}
	if e.now == nil {
		e.now = time.Now
	}
	if e.newID == nil {
		e.newID = newSeqID()
	}
	return e
}

// Registry 暴露注册表，供上层做道具列表等只读查询。
func (e *Engine) Registry() *item.Registry { return e.items }

// lookup 取道具原型。
func (e *Engine) lookup(id string) (item.Item, error) {
	it, ok := e.items.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", item.ErrUnknownItem, id)
	}
	return it, nil
}

// mustOwn 取实例并校验归属。
func (e *Engine) mustOwn(ctx context.Context, owner, instanceID string) (*item.Instance, error) {
	inst, err := e.store.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if inst.Owner != owner {
		return nil, item.ErrNoSuchInstance
	}
	return inst, nil
}

// claim 抢占幂等键；未配置存储或未传键时直接放行。
func (e *Engine) claim(ctx context.Context, key string) error {
	if e.idem == nil || key == "" {
		return nil
	}
	ok, err := e.idem.Claim(ctx, key)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s", item.ErrDuplicateRequest, key)
	}
	return nil
}

// release 业务失败后释放幂等键，允许调用方重试；释放失败静默忽略，不掩盖原始错误。
func (e *Engine) release(ctx context.Context, key string) {
	if e.idem == nil || key == "" {
		return
	}
	_ = e.idem.Release(ctx, key)
}

// publish 发布领域事件。
func (e *Engine) publish(ctx context.Context, k event.Kind, inst *item.Instance, count int64, extra map[string]any) {
	e.bus.Publish(ctx, event.Event{
		Kind:       k,
		Owner:      inst.Owner,
		ItemID:     inst.ItemID,
		InstanceID: inst.ID,
		Count:      count,
		At:         e.now(),
		Extra:      extra,
	})
}

// newSeqID 返回一个内置的自增 ID 生成器（仅适用于单进程；接真实存储时替换为雪花/UUID）。
func newSeqID() func() string {
	var n int64
	return func() string {
		return fmt.Sprintf("inst_%d", atomic.AddInt64(&n, 1))
	}
}
