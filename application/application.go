// Package application 是装配点（组合根）：全系统唯一允许出现"全量依赖"的地方。
// Deps 在这里把端口分发给各领域服务的构造函数；热路径调用只携带 (ctx, owner)，
// 不存在任何工具箱式依赖穿越接缝。
package application

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/linuxea/item-system/domain/behavior"
	"github.com/linuxea/item-system/domain/effect"
	"github.com/linuxea/item-system/domain/event"
	"github.com/linuxea/item-system/domain/expiry"
	"github.com/linuxea/item-system/domain/grant"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/profile"
	"github.com/linuxea/item-system/domain/relation"
	"github.com/linuxea/item-system/domain/repository"
	"github.com/linuxea/item-system/domain/usage"
)

// Deps 全量依赖集合，仅在 New 装配时使用一次。
// 作为组合根，这里允许直接使用 repository 的提供方全量接口。
type Deps struct {
	Templates   repository.TemplateSource
	Instances   repository.InstanceRepo
	Equips      repository.EquipRepo
	Idempotency repository.IdempotencyStore
	Publisher   event.Publisher
	Ledger      effect.Ledger
	Banner      effect.BannerBroadcaster
	Conditions  behavior.ConditionChecker
	Relations   relation.Repo
	Sorts       *profile.SortRegistry
	NewID       func() string
	Now         func() time.Time
	Rand        func(n int64) int64
}

// App 应用门面：聚合各领域服务并提供转发方法。
// 也可以直接使用各 Svc 字段调用具体服务。
type App struct {
	compiler    behavior.Compiler
	deps        Deps
	GrantSvc    *grant.Service
	UseSvc      *usage.Service
	EquipSvc    *profile.EquipService
	ProfileSvc  *profile.ProfileService
	Sorts       *profile.SortRegistry
	ExpirySvc   *expiry.Service
	RelationSvc *relation.Service
}

// New 装配应用：补默认值（排序注册表/时钟/随机源），构建 Registry，
// 组装各领域服务，并把 grantAdapter 晚绑定回 grant.Service。
func New(deps Deps) *App {
	if deps.Sorts == nil {
		deps.Sorts = profile.NewSortRegistry()
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Rand == nil {
		deps.Rand = rand.Int64N
	}

	// grantAdapter 先作为空壳进入 Registry，待 grantSvc 构造完成再回填，
	// 打破 GrantItem 命令 -> grant.Service -> Registry 的构造循环。
	granter := &grantAdapter{}
	registry := behavior.NewRegistry(behavior.Ports{
		Ledger:     deps.Ledger,
		Banner:     deps.Banner,
		Granter:    granter,
		Rand:       deps.Rand,
		Conditions: deps.Conditions,
	})
	grantSvc := grant.NewService(deps.Templates, deps.Instances, deps.Idempotency, deps.Publisher, registry, deps.NewID, deps.Now)
	granter.svc = grantSvc

	useSvc := usage.NewService(deps.Templates, deps.Instances, deps.Idempotency, deps.Publisher, registry, deps.Now)
	equipSvc := profile.NewEquipService(deps.Templates, deps.Instances, deps.Equips, deps.Publisher, registry, deps.Now)
	profileSvc := profile.NewProfileService(deps.Instances, deps.Equips, deps.Templates, registry, deps.Sorts, deps.Now)
	expirySvc := expiry.NewService(deps.Templates, deps.Instances, deps.Equips, deps.Publisher, registry, deps.Now)
	relationSvc := relation.NewService(deps.Relations, deps.Publisher, deps.NewID, deps.Now)

	return &App{
		compiler:    registry,
		deps:        deps,
		GrantSvc:    grantSvc,
		UseSvc:      useSvc,
		EquipSvc:    equipSvc,
		ProfileSvc:  profileSvc,
		Sorts:       deps.Sorts,
		ExpirySvc:   expirySvc,
		RelationSvc: relationSvc,
	}
}

// grantAdapter effect.Granter 的适配器，唯一职责是把"命令再发放"转发回 grant.Service，
// 从而打破 GrantItem -> grant.Service -> Registry 的 import 循环。勿"简化"掉。
type grantAdapter struct {
	svc *grant.Service
}

// GrantEffect 实现再发放端口；svc 为 nil（装配间隙）时静默跳过。
func (a *grantAdapter) GrantEffect(ctx context.Context, owner, templateID string, count int64, reason string) error {
	if a.svc == nil {
		return nil
	}
	_, err := a.svc.Grant(ctx, grant.Request{
		Owner:      owner,
		TemplateID: templateID,
		Count:      count,
		Source:     "effect",
		Reason:     reason,
	})
	return err
}

// Grant 发放道具（转发 grant.Service）。
func (a *App) Grant(ctx context.Context, req grant.Request) (*model.GrantResult, error) {
	return a.GrantSvc.Grant(ctx, req)
}

// Use 使用道具（转发 usage.Service）。
func (a *App) Use(ctx context.Context, req usage.Request) (*model.UseResult, error) {
	return a.UseSvc.Use(ctx, req)
}

// Equip 穿戴道具（转发 profile.EquipService）。
func (a *App) Equip(ctx context.Context, owner, instanceID string) error {
	return a.EquipSvc.Equip(ctx, owner, instanceID)
}

// Unequip 卸下道具（转发 profile.EquipService）。
func (a *App) Unequip(ctx context.Context, owner, instanceID string) error {
	return a.EquipSvc.Unequip(ctx, owner, instanceID)
}

// BuildProfile 构建场景展示快照（转发 profile.ProfileService）。
func (a *App) BuildProfile(ctx context.Context, owner, scene string) (*profile.Snapshot, error) {
	return a.ProfileSvc.Build(ctx, owner, scene)
}

// RunExpiry 批量处理过期道具，返回处理数（转发 expiry.Service）。
func (a *App) RunExpiry(ctx context.Context, limit int) (int, error) {
	return a.ExpirySvc.Run(ctx, limit)
}

// BindRelation 建立双主体关系（转发 relation.Service）。
func (a *App) BindRelation(ctx context.Context, t relation.Type, partyA, partyB string, expireAt *time.Time) (*relation.Relation, error) {
	return a.RelationSvc.Bind(ctx, t, partyA, partyB, expireAt)
}

// BindCP 绑定 CP 的组合流程：建立关系 -> 给双方各发一枚戒指（幂等键挂在关系 ID 上）-> 立即穿戴。
// 任一步失败返回已完成的关系，由调用方决定补偿。
func (a *App) BindCP(ctx context.Context, partyA, partyB, ringTemplateID string) (*relation.Relation, error) {
	rel, err := a.RelationSvc.Bind(ctx, relation.TypeCP, partyA, partyB, nil)
	if err != nil {
		return nil, err
	}
	for i, party := range []string{partyA, partyB} {
		r, err := a.GrantSvc.Grant(ctx, grant.Request{
			Owner:          party,
			TemplateID:     ringTemplateID,
			Count:          1,
			Source:         "relation",
			Reason:         "bind_cp",
			IdempotencyKey: rel.ID + ":ring:" + string(rune('a'+i)),
		})
		if err != nil {
			return rel, err
		}
		if err := a.EquipSvc.Equip(ctx, party, r.InstanceID); err != nil {
			return rel, err
		}
	}
	return rel, nil
}

// DissolveCP 解除 CP 关系（DissolveRelation 的别名，语义化入口）。
func (a *App) DissolveCP(ctx context.Context, relationID string) error {
	return a.DissolveRelation(ctx, relationID)
}

// DissolveRelation 解除关系并联动卸下双方依赖该关系的道具（关系卡/CP 戒指）。
func (a *App) DissolveRelation(ctx context.Context, id string) error {
	rel, err := a.deps.Relations.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := a.RelationSvc.Dissolve(ctx, id); err != nil {
		return err
	}
	return a.unlinkRelationItems(ctx, rel.PartyA, rel.PartyB, rel.Type)
}

// RunRelationExpiry 批量解除到期关系并联动卸下相关道具，返回处理数。
func (a *App) RunRelationExpiry(ctx context.Context, limit int) (int, error) {
	// 先取出到期集合（卸载联动需要双方主体信息），再执行解除。
	expired, err := a.deps.Relations.ListExpired(ctx, a.deps.Now(), limit)
	if err != nil {
		return 0, err
	}
	if n, err := a.RelationSvc.RunExpiry(ctx, limit); err != nil {
		return n, err
	}
	for _, rel := range expired {
		if err := a.unlinkRelationItems(ctx, rel.PartyA, rel.PartyB, rel.Type); err != nil {
			return len(expired), err
		}
	}
	return len(expired), nil
}

// unlinkRelationItems 卸下双方穿戴的、前置条件依赖该类型关系的道具。
// 只扫描关系槽与 CP 戒指槽，避免遍历玩家全部穿戴。
func (a *App) unlinkRelationItems(ctx context.Context, partyA, partyB string, t relation.Type) error {
	for _, owner := range []string{partyA, partyB} {
		records, err := a.deps.Equips.ListByOwner(ctx, owner)
		if err != nil {
			return err
		}
		for _, rec := range records {
			if rec.Slot != model.SlotRelation && rec.Slot != model.SlotCPRing {
				continue
			}
			if !a.itemRequiresRelation(ctx, rec.InstanceID, t) {
				continue
			}
			if err := a.EquipSvc.Unequip(ctx, owner, rec.InstanceID); err != nil {
				return err
			}
		}
	}
	return nil
}

// itemRequiresRelation 判断实例所属模板的穿戴条件是否要求指定关系类型。
// 查询失败按"无依赖"处理，宁可漏卸不可中断联动流程。
func (a *App) itemRequiresRelation(ctx context.Context, instanceID string, t relation.Type) bool {
	inst, err := a.deps.Instances.Get(ctx, instanceID)
	if err != nil {
		return false
	}
	tpl, err := a.deps.Templates.Get(ctx, inst.TemplateID)
	if err != nil {
		return false
	}
	compiled, err := a.compiler.Compile(tpl)
	if err != nil {
		return false
	}
	cond, ok := compiled.Condition()
	return ok && cond.RequiresRelation == string(t)
}
