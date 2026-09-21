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
	Sorts       *profile.SortRegistry
	NewID       func() string
	Now         func() time.Time
	Rand        func(n int64) int64
}

type App struct {
	deps       Deps
	GrantSvc   *grant.Service
	UseSvc     *usage.Service
	EquipSvc   *profile.EquipService
	ProfileSvc *profile.ProfileService
	Sorts      *profile.SortRegistry
	ExpirySvc  *expiry.Service
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
	executor := newEffectExecutor(grantSvc, deps.Ledger, deps.Rand)
	useSvc := usage.NewService(deps.Templates, deps.Instances, deps.Idempotency, deps.Publisher, deps.Registry, executor, deps.Now)
	equipSvc := profile.NewEquipService(deps.Templates, deps.Instances, deps.Equips, deps.Publisher, deps.Registry, deps.Now)
	profileSvc := profile.NewProfileService(deps.Instances, deps.Equips, deps.Templates, deps.Registry, deps.Sorts, deps.Now)
	expirySvc := expiry.NewService(deps.Templates, deps.Instances, deps.Equips, deps.Publisher, deps.Registry, deps.Now)

	return &App{
		deps:       deps,
		GrantSvc:   grantSvc,
		UseSvc:     useSvc,
		EquipSvc:   equipSvc,
		ProfileSvc: profileSvc,
		Sorts:      deps.Sorts,
		ExpirySvc:  expirySvc,
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

type effectExecutor struct {
	grants *grant.Service
	ledger effect.Ledger
	rand   func(n int64) int64
}

func newEffectExecutor(grants *grant.Service, ledger effect.Ledger, rand func(n int64) int64) effect.Executor {
	return &effectExecutor{grants: grants, ledger: ledger, rand: rand}
}

func (e *effectExecutor) Execute(ctx context.Context, owner string, cmds []effect.Command) error {
	for _, cmd := range cmds {
		if err := e.exec(ctx, owner, cmd); err != nil {
			return err
		}
	}
	return nil
}

func (e *effectExecutor) exec(ctx context.Context, owner string, cmd effect.Command) error {
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
