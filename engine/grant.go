package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Linuxea/item-system/event"
	"github.com/Linuxea/item-system/item"
)

// GrantRequest 发放请求。
type GrantRequest struct {
	// Owner 收取玩家。
	Owner string
	// ItemID 道具 ID。
	ItemID string
	// Count 发放数量，须大于 0。
	Count int64
	// IdempotencyKey 幂等键，可选；同一个键重复请求只会成功一次。
	IdempotencyKey string
	// Source 发放来源（gm / shop / activity），仅用于事件与审计。
	Source string
	// Reason 发放原因，仅用于事件与审计。
	Reason string
}

// GrantResult 发放结果。
type GrantResult struct {
	// InstanceID 落地的实例 ID；堆叠合并时为既有实例。
	InstanceID string
	// Count 本次发放数量。
	Count int64
	// Merged 是否合并进了既有实例。
	Merged bool
}

// Grant 发放道具。
//
// 处理顺序：幂等抢占 -> 计算时效与绑定 -> 尝试堆叠合并 -> 否则新建实例 -> 发事件。
// 任一步失败都会释放幂等键，允许调用方重试。
func (e *Engine) Grant(ctx context.Context, req GrantRequest) (*GrantResult, error) {
	if req.Count <= 0 {
		return nil, fmt.Errorf("%w: 发放数量须大于 0", item.ErrInvalidParam)
	}
	it, err := e.lookup(req.ItemID)
	if err != nil {
		return nil, err
	}
	if err := e.claim(ctx, req.IdempotencyKey); err != nil {
		return nil, err
	}

	now := e.now()

	// 时效：实现了 Expirable 的道具，从发放这一刻开始计时。
	var expireAt *time.Time
	if ex, ok := it.(item.Expirable); ok && ex.Duration() > 0 {
		t := now.Add(ex.Duration())
		expireAt = &t
	}

	// 绑定：实现了 Bindable 且声明发放即绑定的道具，落地时就是绑定态。
	bound := false
	if b, ok := it.(item.Bindable); ok {
		bound = b.BindOnGrant()
	}

	// 堆叠合并：仅当道具可堆叠、且本次发放无时效时才尝试。
	// 有时效的实例各自有独立的到期时刻，合并会让其中一份的时效凭空变长或缩短。
	if st, ok := it.(item.Stackable); ok && expireAt == nil && st.MaxStack() > 1 {
		res, merged, err := e.tryMerge(ctx, req, st.MaxStack())
		if err != nil {
			e.release(ctx, req.IdempotencyKey)
			return nil, err
		}
		if merged {
			return res, nil
		}
	}

	inst := &item.Instance{
		ID:         e.newID(),
		ItemID:     req.ItemID,
		Owner:      req.Owner,
		Count:      req.Count,
		Bound:      bound,
		AcquiredAt: now,
		ExpireAt:   expireAt,
	}
	if err := e.store.Create(ctx, inst); err != nil {
		e.release(ctx, req.IdempotencyKey)
		return nil, err
	}

	e.publish(ctx, event.KindGranted, inst, req.Count, map[string]any{
		"source": req.Source,
		"reason": req.Reason,
	})
	return &GrantResult{InstanceID: inst.ID, Count: req.Count}, nil
}

// tryMerge 尝试把本次发放合并进玩家已有的同款实例。
// merged=false 表示没有可合并的目标，调用方应新建实例。
func (e *Engine) tryMerge(ctx context.Context, req GrantRequest, maxStack int64) (*GrantResult, bool, error) {
	owned, err := e.store.ListByOwner(ctx, req.Owner)
	if err != nil {
		return nil, false, err
	}
	for _, cand := range owned {
		if cand.ItemID != req.ItemID || cand.ExpireAt != nil || cand.Equipped {
			continue
		}
		if cand.Count+req.Count > maxStack {
			continue
		}
		expected := cand.Version
		cand.Count += req.Count
		cand.Version++
		if err := e.store.Update(ctx, cand, expected); err != nil {
			// 版本冲突说明这一格刚被别人改过，换下一格试；其余错误直接上报。
			if errors.Is(err, item.ErrVersionConflict) {
				continue
			}
			return nil, false, err
		}
		e.publish(ctx, event.KindGranted, cand, req.Count, map[string]any{
			"source": req.Source,
			"reason": req.Reason,
			"merged": true,
		})
		return &GrantResult{InstanceID: cand.ID, Count: req.Count, Merged: true}, true, nil
	}
	return nil, false, nil
}
