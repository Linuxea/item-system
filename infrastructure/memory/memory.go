package memory

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/linuxea/item-system/application"
	"github.com/linuxea/item-system/domain/behavior"
	"github.com/linuxea/item-system/domain/effect"
	"github.com/linuxea/item-system/domain/event"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/relation"
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
		return &repository.ErrConflict{Entity: "instance " + inst.ID}
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

type BannerRecord struct {
	Owner    string
	Text     string
	Duration time.Duration
}

type BannerRecorder struct {
	mu      sync.Mutex
	records []BannerRecord
}

func NewBannerRecorder() *BannerRecorder {
	return &BannerRecorder{}
}

func (b *BannerRecorder) Broadcast(_ context.Context, owner, text string, duration time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.records = append(b.records, BannerRecord{Owner: owner, Text: text, Duration: duration})
	return nil
}

func (b *BannerRecorder) Records() []BannerRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]BannerRecord(nil), b.records...)
}

type LevelSource struct {
	mu     sync.Mutex
	levels map[string]int64
}

func NewLevelSource() *LevelSource {
	return &LevelSource{levels: map[string]int64{}}
}

func (s *LevelSource) SetLevel(owner string, level int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.levels[owner] = level
}

func (s *LevelSource) Level(owner string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.levels[owner]
}

type RelationRepo struct {
	mu        sync.Mutex
	relations map[string]*relation.Relation
	now       func() time.Time
}

func NewRelationRepo() *RelationRepo {
	return &RelationRepo{relations: map[string]*relation.Relation{}, now: time.Now}
}

func (r *RelationRepo) UseClock(now func() time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.now = now
}

func (r *RelationRepo) Save(_ context.Context, rel *relation.Relation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.relations[rel.ID]; exists {
		return &repository.ErrConflict{Entity: "relation " + rel.ID}
	}
	cp := *rel
	r.relations[rel.ID] = &cp
	return nil
}

func (r *RelationRepo) Get(_ context.Context, id string) (*relation.Relation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rel, ok := r.relations[id]
	if !ok {
		return nil, relation.ErrNotFound
	}
	cp := *rel
	return &cp, nil
}

func (r *RelationRepo) FindActive(_ context.Context, owner string, t relation.Type) (*relation.Relation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	for _, rel := range r.relations {
		if rel.Type == t && rel.Status == relation.StatusActive && rel.Involves(owner) && !rel.Expired(now) {
			cp := *rel
			return &cp, nil
		}
	}
	return nil, relation.ErrNotFound
}

func (r *RelationRepo) ListExpired(_ context.Context, before time.Time, limit int) ([]*relation.Relation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*relation.Relation
	for _, rel := range r.relations {
		if rel.Status == relation.StatusActive && rel.Expired(before) {
			cp := *rel
			out = append(out, &cp)
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *RelationRepo) Update(_ context.Context, rel *relation.Relation, expectVersion int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.relations[rel.ID]
	if !ok {
		return relation.ErrNotFound
	}
	if cur.Version != expectVersion {
		return &repository.ErrVersionConflict{Entity: "relation " + rel.ID}
	}
	cp := *rel
	r.relations[rel.ID] = &cp
	return nil
}

type ConditionChecker struct {
	Levels    *LevelSource
	Relations *RelationRepo
}

func NewConditionChecker(levels *LevelSource) *ConditionChecker {
	return &ConditionChecker{Levels: levels}
}

func (c *ConditionChecker) Satisfied(ctx context.Context, owner string, cond behavior.Condition) (bool, error) {
	if cond.MinLevel > 0 && c.Levels.Level(owner) < cond.MinLevel {
		return false, nil
	}
	if cond.RequiresRelation != "" {
		if c.Relations == nil {
			return false, nil
		}
		if _, err := c.Relations.FindActive(ctx, owner, relation.Type(cond.RequiresRelation)); err != nil {
			return false, nil
		}
	}
	return true, nil
}

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
	Levels      *LevelSource
	Relations   *RelationRepo
	Banner      *BannerRecorder
}

func NewStack(tpls ...*model.ItemTemplate) *Stack {
	bus := NewEventBus()
	instances := NewInstanceRepo()
	templates := NewTemplateSource(tpls...)
	equips := NewEquipRepo()
	idem := NewIdempotencyStore()
	ledger := NewLedger()
	levels := NewLevelSource()
	relations := NewRelationRepo()
	banner := NewBannerRecorder()
	app := application.New(application.Deps{
		Templates:   templates,
		Instances:   instances,
		Equips:      equips,
		Idempotency: idem,
		Publisher:   bus,
		Ledger:      ledger,
		Banner:      banner,
		Conditions:  &ConditionChecker{Levels: levels, Relations: relations},
		Relations:   relations,
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
		Levels:      levels,
		Relations:   relations,
		Banner:      banner,
	}
}
