package memory

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/linuxea/item-system/application"
	"github.com/linuxea/item-system/domain/effect"
	"github.com/linuxea/item-system/domain/event"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/repository"
)

type TemplateSource struct {
	mu        sync.RWMutex
	templates map[string]*model.ItemTemplate
}

func NewTemplateSource(tpls ...*model.ItemTemplate) *TemplateSource {
	m := map[string]*model.ItemTemplate{}
	for _, t := range tpls {
		cp := *t
		m[t.ID] = &cp
	}
	return &TemplateSource{templates: m}
}

func (s *TemplateSource) Get(_ context.Context, id string) (*model.ItemTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.templates[id]
	if !ok {
		return nil, &repository.ErrNotFound{Entity: "template " + id}
	}
	cp := *t
	return &cp, nil
}

type InstanceRepo struct {
	mu        sync.Mutex
	instances map[string]*model.ItemInstance
	counter   int64
}

func NewInstanceRepo() *InstanceRepo {
	return &InstanceRepo{instances: map[string]*model.ItemInstance{}}
}

func (r *InstanceRepo) Save(_ context.Context, inst *model.ItemInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.instances[inst.ID]; exists {
		return &repository.ErrNotFound{Entity: "duplicate instance " + inst.ID}
	}
	cp := *inst
	r.instances[inst.ID] = &cp
	return nil
}

func (r *InstanceRepo) Get(_ context.Context, id string) (*model.ItemInstance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inst, ok := r.instances[id]
	if !ok {
		return nil, &repository.ErrNotFound{Entity: "instance " + id}
	}
	cp := *inst
	return &cp, nil
}

func (r *InstanceRepo) ListByOwner(_ context.Context, owner string) ([]*model.ItemInstance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*model.ItemInstance
	for _, inst := range r.instances {
		if inst.Owner == owner {
			cp := *inst
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AcquiredAt.Before(out[j].AcquiredAt) })
	return out, nil
}

func (r *InstanceRepo) Update(_ context.Context, inst *model.ItemInstance, expectVersion int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.instances[inst.ID]
	if !ok {
		return &repository.ErrNotFound{Entity: "instance " + inst.ID}
	}
	if cur.Version != expectVersion {
		return &repository.ErrVersionConflict{Entity: "instance " + inst.ID}
	}
	cp := *inst
	r.instances[inst.ID] = &cp
	return nil
}

func (r *InstanceRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.instances, id)
	return nil
}

func (r *InstanceRepo) ListExpired(_ context.Context, before time.Time, limit int) ([]*model.ItemInstance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*model.ItemInstance
	for _, inst := range r.instances {
		if inst.ExpireAt != nil && inst.ExpireAt.Before(before) {
			cp := *inst
			out = append(out, &cp)
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

type EquipRepo struct {
	mu      sync.Mutex
	records map[string]model.EquipRecord
}

func NewEquipRepo() *EquipRepo {
	return &EquipRepo{records: map[string]model.EquipRecord{}}
}

func keyOf(owner string, slot model.SlotType, instanceID string) string {
	return owner + "|" + string(slot) + "|" + instanceID
}

func (r *EquipRepo) ListByOwner(_ context.Context, owner string) ([]model.EquipRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []model.EquipRecord
	for _, rec := range r.records {
		if rec.Owner == owner {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (r *EquipRepo) ListBySlot(_ context.Context, owner string, slot model.SlotType) ([]model.EquipRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []model.EquipRecord
	for _, rec := range r.records {
		if rec.Owner == owner && rec.Slot == slot {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (r *EquipRepo) Save(_ context.Context, rec model.EquipRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[keyOf(rec.Owner, rec.Slot, rec.InstanceID)] = rec
	return nil
}

func (r *EquipRepo) Delete(_ context.Context, owner string, slot model.SlotType, instanceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.records, keyOf(owner, slot, instanceID))
	return nil
}

func (r *EquipRepo) DeleteByInstance(_ context.Context, instanceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, rec := range r.records {
		if rec.InstanceID == instanceID {
			delete(r.records, key)
		}
	}
	return nil
}

type IdempotencyStore struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func NewIdempotencyStore() *IdempotencyStore {
	return &IdempotencyStore{seen: map[string]struct{}{}}
}

func (s *IdempotencyStore) Claim(_ context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seen[key]; ok {
		return false, nil
	}
	s.seen[key] = struct{}{}
	return true, nil
}

func (s *IdempotencyStore) Release(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.seen, key)
	return nil
}

type EventBus struct {
	mu       sync.Mutex
	handlers []func(event.Event)
	events   []event.Event
}

func NewEventBus() *EventBus {
	return &EventBus{}
}

func (b *EventBus) Subscribe(h func(event.Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, h)
}

func (b *EventBus) Recorded() []event.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]event.Event(nil), b.events...)
}

func (b *EventBus) Publish(events ...event.Event) {
	b.mu.Lock()
	b.events = append(b.events, events...)
	handlers := append([]func(event.Event){}, b.handlers...)
	b.mu.Unlock()
	for _, e := range events {
		for _, h := range handlers {
			h(e)
		}
	}
}

type Ledger struct {
	mu      sync.Mutex
	balance map[string]map[string]int64
}

func NewLedger() *Ledger {
	return &Ledger{balance: map[string]map[string]int64{}}
}

func (l *Ledger) Add(_ context.Context, owner, currency string, amount int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.balance[owner] == nil {
		l.balance[owner] = map[string]int64{}
	}
	l.balance[owner][currency] += amount
	return nil
}

func (l *Ledger) Balance(owner, currency string) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.balance[owner][currency]
}

var _ effect.Ledger = (*Ledger)(nil)

type IDGenerator struct {
	mu     sync.Mutex
	n      int64
	prefix string
}

func NewIDGenerator(prefix string) *IDGenerator {
	return &IDGenerator{prefix: prefix}
}

func (g *IDGenerator) Next() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return g.prefix + strconv.FormatInt(g.n, 10)
}

type Stack struct {
	App         *application.App
	Bus         *EventBus
	Ledger      *Ledger
	Instances   *InstanceRepo
	Templates   *TemplateSource
	Equips      *EquipRepo
	Idempotency *IdempotencyStore
}

func NewStack(tpls ...*model.ItemTemplate) *Stack {
	bus := NewEventBus()
	instances := NewInstanceRepo()
	templates := NewTemplateSource(tpls...)
	equips := NewEquipRepo()
	idem := NewIdempotencyStore()
	ledger := NewLedger()
	app := application.New(application.Deps{
		Templates:   templates,
		Instances:   instances,
		Equips:      equips,
		Idempotency: idem,
		Publisher:   bus,
		Ledger:      ledger,
		NewID:       NewIDGenerator("inst_").Next,
	})
	return &Stack{
		App:         app,
		Bus:         bus,
		Ledger:      ledger,
		Instances:   instances,
		Templates:   templates,
		Equips:      equips,
		Idempotency: idem,
	}
}
