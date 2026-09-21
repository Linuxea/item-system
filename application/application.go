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

type App struct {
	registry    *behavior.Registry
	deps        Deps
	GrantSvc    *grant.Service
	UseSvc      *usage.Service
	EquipSvc    *profile.EquipService
	ProfileSvc  *profile.ProfileService
	Sorts       *profile.SortRegistry
	ExpirySvc   *expiry.Service
	RelationSvc *relation.Service
}

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

	granter := &grantAdapter{}
	registry := behavior.NewRegistry(behavior.Ports{
		Ledger:  deps.Ledger,
		Banner:  deps.Banner,
		Granter: granter,
		Rand:    deps.Rand,
	})
	grantSvc := grant.NewService(deps.Templates, deps.Instances, deps.Idempotency, deps.Publisher, registry, deps.NewID, deps.Now)
	granter.svc = grantSvc

	useSvc := usage.NewService(deps.Templates, deps.Instances, deps.Idempotency, deps.Publisher, registry, deps.Now)
	equipSvc := profile.NewEquipService(deps.Templates, deps.Instances, deps.Equips, deps.Publisher, registry, deps.Conditions, deps.Now)
	profileSvc := profile.NewProfileService(deps.Instances, deps.Equips, deps.Templates, registry, deps.Sorts, deps.Now)
	expirySvc := expiry.NewService(deps.Templates, deps.Instances, deps.Equips, deps.Publisher, registry, deps.Now)
	relationSvc := relation.NewService(deps.Relations, deps.Publisher, deps.NewID, deps.Now)

	return &App{
		registry:    registry,
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

type grantAdapter struct {
	svc *grant.Service
}

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

func (a *App) Grant(ctx context.Context, req grant.Request) (*model.GrantResult, error) {
	return a.GrantSvc.Grant(ctx, req)
}

func (a *App) Use(ctx context.Context, req usage.Request) (*model.UseResult, error) {
	return a.UseSvc.Use(ctx, req)
}

func (a *App) Equip(ctx context.Context, owner, instanceID string) error {
	return a.EquipSvc.Equip(ctx, owner, instanceID)
}

func (a *App) Unequip(ctx context.Context, owner, instanceID string) error {
	return a.EquipSvc.Unequip(ctx, owner, instanceID)
}

func (a *App) BuildProfile(ctx context.Context, owner, scene string) (*profile.Snapshot, error) {
	return a.ProfileSvc.Build(ctx, owner, scene)
}

func (a *App) RunExpiry(ctx context.Context, limit int) (int, error) {
	return a.ExpirySvc.Run(ctx, limit)
}

func (a *App) BindRelation(ctx context.Context, t relation.Type, partyA, partyB string, expireAt *time.Time) (*relation.Relation, error) {
	return a.RelationSvc.Bind(ctx, t, partyA, partyB, expireAt)
}

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

func (a *App) DissolveCP(ctx context.Context, relationID string) error {
	return a.DissolveRelation(ctx, relationID)
}

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

func (a *App) RunRelationExpiry(ctx context.Context, limit int) (int, error) {
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

func (a *App) itemRequiresRelation(ctx context.Context, instanceID string, t relation.Type) bool {
	inst, err := a.deps.Instances.Get(ctx, instanceID)
	if err != nil {
		return false
	}
	tpl, err := a.deps.Templates.Get(ctx, inst.TemplateID)
	if err != nil {
		return false
	}
	compiled, err := a.registry.Compile(tpl)
	if err != nil {
		return false
	}
	cond, ok := compiled.Condition()
	return ok && cond.RequiresRelation == string(t)
}
