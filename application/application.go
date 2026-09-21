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
	Registry    *behavior.Registry
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
	if deps.Registry == nil {
		deps.Registry = behavior.NewRegistry()
	}
	if deps.Sorts == nil {
		deps.Sorts = profile.NewSortRegistry()
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Rand == nil {
		deps.Rand = rand.Int64N
	}

	grantSvc := grant.NewService(deps.Templates, deps.Instances, deps.Idempotency, deps.Publisher, deps.Registry, deps.NewID, deps.Now)
	executor := newEffectExecutor(grantSvc, deps.Ledger, deps.Banner, deps.Rand)
	useSvc := usage.NewService(deps.Templates, deps.Instances, deps.Idempotency, deps.Publisher, deps.Registry, executor, deps.Now)
	equipSvc := profile.NewEquipService(deps.Templates, deps.Instances, deps.Equips, deps.Publisher, deps.Registry, deps.Conditions, deps.Now)
	profileSvc := profile.NewProfileService(deps.Instances, deps.Equips, deps.Templates, deps.Registry, deps.Sorts, deps.Now)
	expirySvc := expiry.NewService(deps.Templates, deps.Instances, deps.Equips, deps.Publisher, deps.Registry, deps.Now)
	relationSvc := relation.NewService(deps.Relations, deps.Publisher, deps.NewID, deps.Now)

	return &App{
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
	compiled, err := a.deps.Registry.Compile(tpl)
	if err != nil {
		return false
	}
	cond, ok := compiled.Condition()
	return ok && cond.RequiresRelation == string(t)
}

type effectExecutor struct {
	grants *grant.Service
	ledger effect.Ledger
	banner effect.BannerBroadcaster
	rand   func(n int64) int64
}

func newEffectExecutor(grants *grant.Service, ledger effect.Ledger, banner effect.BannerBroadcaster, rand func(n int64) int64) effect.Executor {
	return &effectExecutor{grants: grants, ledger: ledger, banner: banner, rand: rand}
}

func (e *effectExecutor) Execute(ctx context.Context, owner string, params map[string]any, cmds []effect.Command) error {
	for _, cmd := range cmds {
		if err := e.exec(ctx, owner, params, cmd); err != nil {
			return err
		}
	}
	return nil
}

func (e *effectExecutor) exec(ctx context.Context, owner string, params map[string]any, cmd effect.Command) error {
	switch c := cmd.(type) {
	case effect.AddCurrency:
		if e.ledger == nil {
			return nil
		}
		return e.ledger.Add(ctx, owner, c.Currency, c.Amount)
	case effect.GrantItem:
		_, err := e.grants.Grant(ctx, grant.Request{
			Owner:      owner,
			TemplateID: c.TemplateID,
			Count:      c.Count,
			Source:     "effect",
			Reason:     effect.KindGrantItem,
		})
		return err
	case effect.RandomGrant:
		entry, ok := e.pick(c.Entries)
		if !ok {
			return nil
		}
		_, err := e.grants.Grant(ctx, grant.Request{
			Owner:      owner,
			TemplateID: entry.TemplateID,
			Count:      entry.Count,
			Source:     "effect",
			Reason:     effect.KindRandomGrant,
		})
		return err
	case effect.BroadcastBanner:
		text, _ := params[c.TextParam].(string)
		if text == "" {
			return effect.ErrMissingParam
		}
		if e.banner == nil {
			return nil
		}
		return e.banner.Broadcast(ctx, owner, text, c.Duration)
	default:
		return nil
	}
}

func (e *effectExecutor) pick(entries []effect.RandomEntry) (effect.RandomEntry, bool) {
	if len(entries) == 0 {
		return effect.RandomEntry{}, false
	}
	var total int64
	for _, en := range entries {
		total += en.Weight
	}
	if total <= 0 {
		return effect.RandomEntry{}, false
	}
	n := e.rand(total)
	for _, en := range entries {
		if n < en.Weight {
			return en, true
		}
		n -= en.Weight
	}
	return entries[len(entries)-1], true
}
