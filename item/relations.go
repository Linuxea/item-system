// relations.go 双主体关系流程：建立/解除/到期由库存层驱动（关系模型在 relation 包），
// 解除时联动卸下依赖该关系的道具（RelationGated 声明依赖）。
// BindCP 是"建立关系 + 成对发放戒指 + 自动穿戴"的组合流程。
package item

import (
	"context"
	"errors"
	"time"

	"github.com/Linuxea/item-system/event"
	"github.com/Linuxea/item-system/relation"
)

// BindRelation 建立关系：禁止自绑，且任一方在此类型下已有生效关系即拒绝（独占语义）。
func (inv *Inventory) BindRelation(ctx context.Context, t relation.Type, partyA, partyB string, expireAt *time.Time) (*relation.Relation, error) {
	if inv.Relations == nil {
		return nil, ErrNotConfigured
	}
	if partyA == partyB {
		return nil, relation.ErrSelfBinding
	}
	for _, party := range []string{partyA, partyB} {
		if _, err := inv.Relations.FindActive(ctx, party, t); !errors.Is(err, relation.ErrNotFound) {
			return nil, relation.ErrAlreadyBound
		}
	}

	r := &relation.Relation{
		ID:        inv.NewID(),
		Type:      t,
		PartyA:    partyA,
		PartyB:    partyB,
		Status:    relation.StatusActive,
		CreatedAt: inv.Now(),
		ExpireAt:  expireAt,
	}
	if err := inv.Relations.Save(ctx, r); err != nil {
		return nil, err
	}

	inv.Events.Publish(relation.Bound{
		Base:       event.Base{At: inv.Now()},
		RelationID: r.ID,
		Type:       r.Type,
		PartyA:     r.PartyA,
		PartyB:     r.PartyB,
	})
	return r, nil
}

// DissolveRelation 主动解除关系（版本 CAS，幂等拒绝重复解除），并联动卸下
// 双方穿戴的、前置依赖该类型关系的道具。
func (inv *Inventory) DissolveRelation(ctx context.Context, id string) error {
	if inv.Relations == nil {
		return ErrNotConfigured
	}
	rel, err := inv.Relations.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := inv.dissolveRelation(ctx, rel, false); err != nil {
		return err
	}
	return inv.unlinkRelationItems(ctx, rel.PartyA, rel.PartyB, rel.Type)
}

// RunRelationExpiry 批量解除到期关系并联动卸下相关道具，返回处理数。
func (inv *Inventory) RunRelationExpiry(ctx context.Context, limit int) (int, error) {
	if inv.Relations == nil {
		return 0, ErrNotConfigured
	}
	// 先取出到期集合（联动卸下需要双方主体信息），再逐个解除。
	expired, err := inv.Relations.ListExpired(ctx, inv.Now(), limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, rel := range expired {
		if err := inv.dissolveRelation(ctx, rel, true); err != nil {
			return processed, err
		}
		if err := inv.unlinkRelationItems(ctx, rel.PartyA, rel.PartyB, rel.Type); err != nil {
			return processed + 1, err
		}
		processed++
	}
	return processed, nil
}

// dissolveRelation 把关系置为已解除并清空时效（版本 CAS），发布解除事件。
func (inv *Inventory) dissolveRelation(ctx context.Context, rel *relation.Relation, byExpiry bool) error {
	if rel.Status == relation.StatusDissolved {
		return relation.ErrDissolved
	}
	expect := rel.Version
	rel.Status = relation.StatusDissolved
	rel.ExpireAt = nil
	rel.BumpVersion()
	if err := inv.Relations.Update(ctx, rel, expect); err != nil {
		return err
	}
	inv.Events.Publish(relation.Dissolved{
		Base:       event.Base{At: inv.Now()},
		RelationID: rel.ID,
		Type:       rel.Type,
		PartyA:     rel.PartyA,
		PartyB:     rel.PartyB,
		ByExpiry:   byExpiry,
	})
	return nil
}

// BindCP 绑定 CP 的组合流程：建立关系 -> 给双方各发一枚戒指
// （幂等键挂在关系 ID 上）-> 立即穿戴。任一步失败返回已完成的关系，
// 由调用方决定补偿。
func (inv *Inventory) BindCP(ctx context.Context, partyA, partyB, ringDefID string) (*relation.Relation, error) {
	rel, err := inv.BindRelation(ctx, relation.TypeCP, partyA, partyB, nil)
	if err != nil {
		return nil, err
	}
	for i, party := range []string{partyA, partyB} {
		r, err := inv.Grant(ctx, GrantRequest{
			Owner:          party,
			DefID:          ringDefID,
			Count:          1,
			Source:         "relation",
			Reason:         "bind_cp",
			IdempotencyKey: rel.ID + ":ring:" + string(rune('a'+i)),
		})
		if err != nil {
			return rel, err
		}
		if err := inv.Equip(ctx, party, r.InstanceID); err != nil {
			return rel, err
		}
	}
	return rel, nil
}

// DissolveCP 解除 CP 关系（DissolveRelation 的语义化别名）。
func (inv *Inventory) DissolveCP(ctx context.Context, relationID string) error {
	return inv.DissolveRelation(ctx, relationID)
}

// unlinkRelationItems 卸下双方穿戴的、前置条件依赖该类型关系的道具
// （关系卡、CP 戒指等 RelationGated 道具）。
func (inv *Inventory) unlinkRelationItems(ctx context.Context, partyA, partyB string, t relation.Type) error {
	for _, owner := range []string{partyA, partyB} {
		records, err := inv.Equips.ListByOwner(ctx, owner)
		if err != nil {
			return err
		}
		for _, rec := range records {
			inst, err := inv.Instances.Get(ctx, rec.InstanceID)
			if err != nil {
				continue
			}
			def, err := inv.Defs.Get(ctx, inst.DefID)
			if err != nil {
				continue
			}
			if gated, ok := def.(RelationGated); ok && gated.RequiresRelation() == t {
				if err := inv.Unequip(ctx, owner, rec.InstanceID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
