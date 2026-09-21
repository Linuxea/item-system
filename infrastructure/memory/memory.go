// Package memory 提供领域端口的全套内存实现：
// 各仓储、事件总线、账本、飘屏记录器、等级源等，以及一键装配的 Stack。
// 既用于测试，也作为未来 MySQL/Redis 实现的行为对照。
// 所有仓储读写均做值拷贝，避免外部拿到内部指针后绕过仓储修改数据。
package memory

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Linuxea/item-system/application"
	"github.com/Linuxea/item-system/domain/behavior"
	"github.com/Linuxea/item-system/domain/effect"
	"github.com/Linuxea/item-system/domain/event"
	"github.com/Linuxea/item-system/domain/model"
	"github.com/Linuxea/item-system/domain/relation"
	"github.com/Linuxea/item-system/domain/repository"
)

// TemplateSource 模板只读源的内存实现。
type TemplateSource struct {
	mu        sync.RWMutex
	templates map[string]*model.ItemTemplate
}

// NewTemplateSource 以给定模板集构建（存入前拷贝，与调用方解耦）。
func NewTemplateSource(tpls ...*model.ItemTemplate) *TemplateSource {
	m := map[string]*model.ItemTemplate{}
	for _, t := range tpls {
		cp := *t
		m[t.ID] = &cp
	}
	return &TemplateSource{templates: m}
}

// Get 按 ID 取模板（返回拷贝）。
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

// InstanceRepo 道具实例仓储的内存实现。
// Update 严格执行乐观锁：期望版本与当前版本不一致即返回 ErrVersionConflict。
type InstanceRepo struct {
	mu        sync.Mutex
	instances map[string]*model.ItemInstance
	counter   int64
}

// NewInstanceRepo 创建空的实例仓储。
func NewInstanceRepo() *InstanceRepo {
	return &InstanceRepo{instances: map[string]*model.ItemInstance{}}
}

// Save 新建实例；ID 已存在视为冲突（ErrConflict）。
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

// Get 按 ID 取实例（返回拷贝）。
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

// ListByOwner 列出玩家全部实例，按获取时间升序（保证堆叠合并顺序确定）。
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

// Update 以乐观锁条件更新实例（CAS）：expectVersion 必须等于当前版本。
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

// Delete 删除实例（幂等，不存在也返回成功）。
func (r *InstanceRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.instances, id)
	return nil
}

// ListExpired 取 before 时刻前到期的一批实例，limit 控制批量（<=0 不限）。
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

// EquipRepo 穿戴记录仓储的内存实现，以 "owner|slot|instanceID" 复合键存储。
type EquipRepo struct {
	mu      sync.Mutex
	records map[string]model.EquipRecord
}

// NewEquipRepo 创建空的穿戴记录仓储。
func NewEquipRepo() *EquipRepo {
	return &EquipRepo{records: map[string]model.EquipRecord{}}
}

// keyOf 生成穿戴记录的复合存储键。
func keyOf(owner string, slot model.SlotType, instanceID string) string {
	return owner + "|" + string(slot) + "|" + instanceID
}

// ListByOwner 列出玩家全部穿戴记录。
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

// ListBySlot 列出玩家在指定槽位的穿戴记录（用于容量检查）。
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

// Save 保存穿戴记录（同键覆盖）。
func (r *EquipRepo) Save(_ context.Context, rec model.EquipRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[keyOf(rec.Owner, rec.Slot, rec.InstanceID)] = rec
	return nil
}

// Delete 删除指定键的穿戴记录（幂等）。
func (r *EquipRepo) Delete(_ context.Context, owner string, slot model.SlotType, instanceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.records, keyOf(owner, slot, instanceID))
	return nil
}

// DeleteByInstance 删除某实例的全部穿戴记录（跨槽位清理，供过期处理使用）。
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

// IdempotencyStore 幂等键存储的内存实现：已占用的键 Claim 返回 false。
type IdempotencyStore struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

// NewIdempotencyStore 创建空的幂等存储。
func NewIdempotencyStore() *IdempotencyStore {
	return &IdempotencyStore{seen: map[string]struct{}{}}
}

// Claim 抢占幂等键，成功返回 true；已被占用返回 false。
func (s *IdempotencyStore) Claim(_ context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seen[key]; ok {
		return false, nil
	}
	s.seen[key] = struct{}{}
	return true, nil
}

// Release 释放幂等键（业务失败回滚时调用）。
func (s *IdempotencyStore) Release(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.seen, key)
	return nil
}

// EventBus 事件总线的内存实现：同步分发 + 全量留痕。
// Recorded 保存历史事件，供测试断言事件发布行为。
type EventBus struct {
	mu       sync.Mutex
	handlers []func(event.Event)
	events   []event.Event
}

// NewEventBus 创建事件总线。
func NewEventBus() *EventBus {
	return &EventBus{}
}

// Subscribe 订阅事件；回调在 Publish 内同步执行。
func (b *EventBus) Subscribe(h func(event.Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, h)
}

// Recorded 返回历史事件拷贝（测试断言入口）。
func (b *EventBus) Recorded() []event.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]event.Event(nil), b.events...)
}

// Publish 先留痕，再对订阅者快照做同步分发；
// 回调在锁外执行，避免回调内再触发 Publish 造成死锁。
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

// Ledger 货币账本的内存实现，balance[owner][currency] 累加。
type Ledger struct {
	mu      sync.Mutex
	balance map[string]map[string]int64
}

// NewLedger 创建空账本。
func NewLedger() *Ledger {
	return &Ledger{balance: map[string]map[string]int64{}}
}

// Add 累加玩家某币种余额。
func (l *Ledger) Add(_ context.Context, owner, currency string, amount int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.balance[owner] == nil {
		l.balance[owner] = map[string]int64{}
	}
	l.balance[owner][currency] += amount
	return nil
}

// Balance 查询玩家某币种余额（测试断言入口）。
func (l *Ledger) Balance(owner, currency string) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.balance[owner][currency]
}

// 编译期断言：Ledger 满足 effect.Ledger 端口（结构化类型适配）。
var _ effect.Ledger = (*Ledger)(nil)

// BannerRecord 一次飘屏广播的记录。
type BannerRecord struct {
	Owner    string
	Text     string
	Duration time.Duration
}

// BannerRecorder 飘屏端口的内存实现：只记录不广播，供测试断言。
type BannerRecorder struct {
	mu      sync.Mutex
	records []BannerRecord
}

// NewBannerRecorder 创建飘屏记录器。
func NewBannerRecorder() *BannerRecorder {
	return &BannerRecorder{}
}

// Broadcast 记录一次广播。
func (b *BannerRecorder) Broadcast(_ context.Context, owner, text string, duration time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.records = append(b.records, BannerRecord{Owner: owner, Text: text, Duration: duration})
	return nil
}

// Records 返回广播历史拷贝（测试断言入口）。
func (b *BannerRecorder) Records() []BannerRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]BannerRecord(nil), b.records...)
}

// LevelSource 玩家等级源的内存实现，供条件校验（min_level）查询。
type LevelSource struct {
	mu     sync.Mutex
	levels map[string]int64
}

// NewLevelSource 创建等级源（默认所有人 0 级）。
func NewLevelSource() *LevelSource {
	return &LevelSource{levels: map[string]int64{}}
}

// SetLevel 设置玩家等级（测试入口）。
func (s *LevelSource) SetLevel(owner string, level int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.levels[owner] = level
}

// Level 查询玩家等级，未设置为 0。
func (s *LevelSource) Level(owner string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.levels[owner]
}

// RelationRepo 关系仓储的内存实现。
// FindActive 内部按自身时钟过滤已过期关系，因此测试需用 UseClock 注入固定时钟。
type RelationRepo struct {
	mu        sync.Mutex
	relations map[string]*relation.Relation
	now       func() time.Time
}

// NewRelationRepo 创建空的关系仓储。
func NewRelationRepo() *RelationRepo {
	return &RelationRepo{relations: map[string]*relation.Relation{}, now: time.Now}
}

// UseClock 替换内部时钟（关系时效测试专用接缝）。
func (r *RelationRepo) UseClock(now func() time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.now = now
}

// Save 新建关系；ID 已存在视为冲突。
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

// Get 按 ID 取关系（返回拷贝）。
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

// FindActive 查玩家在某类型下的生效关系；过期即视同不存在。
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

// ListExpired 取 before 时刻前到期的生效关系，limit 控制批量（<=0 不限）。
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

// Update 以乐观锁条件更新关系（CAS）。
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

// ConditionChecker 条件校验端口的内存实现：
// MinLevel 查等级源，RequiresRelation 查关系仓储（无仓储时直接不满足）。
type ConditionChecker struct {
	Levels    *LevelSource
	Relations *RelationRepo
}

// NewConditionChecker 以等级源构建条件校验器。
func NewConditionChecker(levels *LevelSource) *ConditionChecker {
	return &ConditionChecker{Levels: levels}
}

// Satisfied 依次校验等级与关系前置条件。
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

// IDGenerator 线程安全的递增 ID 生成器（prefix + 序号）。
type IDGenerator struct {
	mu     sync.Mutex
	n      int64
	prefix string
}

// NewIDGenerator 创建带前缀的 ID 生成器。
func NewIDGenerator(prefix string) *IDGenerator {
	return &IDGenerator{prefix: prefix}
}

// Next 生成下一个 ID。
func (g *IDGenerator) Next() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return g.prefix + strconv.FormatInt(g.n, 10)
}

// Stack 一键装配的完整内存栈：App 与全部基础设施组件。
// 测试与演示直接使用；字段开放访问便于断言与定制（如 SetLevel、Subscribe）。
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

// NewStack 装配全栈：创建全部内存组件并注入 application.Deps。
// 自定义时钟/随机源时请手动组装 Deps（模式见 application/mount_test.go）。
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
