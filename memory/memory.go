// Package memory 提供道具系统各端口的内存实现：道具定义注册表、各仓储、
// 事件总线、账本、飘屏记录器、改名记录器、等级源等，以及一键装配的 Stack。
// 既用于测试，也作为未来 MySQL/Redis 实现的行为对照。
// 所有仓储读写均做值拷贝，避免外部拿到内部指针后绕过仓储修改数据。
package memory

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Linuxea/item-system/event"
	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/relation"
)

// DefRegistry 道具定义注册表：ID -> 定义。可在装配后继续 Register
// （定义持有外部端口，通常需要先建好基础设施再注册）。
type DefRegistry struct {
	mu   sync.RWMutex
	defs map[string]item.Def
}

// NewDefRegistry 创建空注册表。
func NewDefRegistry() *DefRegistry {
	return &DefRegistry{defs: map[string]item.Def{}}
}

// Register 登记道具定义（同 ID 覆盖）。
func (r *DefRegistry) Register(defs ...item.Def) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range defs {
		r.defs[d.DefID()] = d
	}
}

// Get 按标识取道具定义；未登记返回 ErrNotFound。
func (r *DefRegistry) Get(_ context.Context, defID string) (item.Def, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.defs[defID]
	if !ok {
		return nil, &item.ErrNotFound{Entity: "def " + defID}
	}
	return d, nil
}

// 编译期断言：DefRegistry 满足 item.DefSource。
var _ item.DefSource = (*DefRegistry)(nil)

// InstanceRepo 道具实例仓储的内存实现。
// Update 严格执行乐观锁：期望版本与当前版本不一致即返回 ErrVersionConflict。
type InstanceRepo struct {
	mu        sync.Mutex
	instances map[string]*item.Instance
}

// NewInstanceRepo 创建空的实例仓储。
func NewInstanceRepo() *InstanceRepo {
	return &InstanceRepo{instances: map[string]*item.Instance{}}
}

// Save 新建实例；ID 已存在视为冲突（ErrConflict）。
func (r *InstanceRepo) Save(_ context.Context, inst *item.Instance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.instances[inst.ID]; exists {
		return &item.ErrConflict{Entity: "instance " + inst.ID}
	}
	cp := *inst
	r.instances[inst.ID] = &cp
	return nil
}

// Get 按 ID 取实例（返回拷贝）。
func (r *InstanceRepo) Get(_ context.Context, id string) (*item.Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inst, ok := r.instances[id]
	if !ok {
		return nil, &item.ErrNotFound{Entity: "instance " + id}
	}
	cp := *inst
	return &cp, nil
}

// ListByOwner 列出玩家全部实例，按获取时间升序（保证堆叠合并顺序确定）。
func (r *InstanceRepo) ListByOwner(_ context.Context, owner string) ([]*item.Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*item.Instance
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
func (r *InstanceRepo) Update(_ context.Context, inst *item.Instance, expectVersion int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.instances[inst.ID]
	if !ok {
		return &item.ErrNotFound{Entity: "instance " + inst.ID}
	}
	if cur.Version != expectVersion {
		return &item.ErrVersionConflict{Entity: "instance " + inst.ID}
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
func (r *InstanceRepo) ListExpired(_ context.Context, before time.Time, limit int) ([]*item.Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*item.Instance
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
	records map[string]item.EquipRecord
}

// NewEquipRepo 创建空的穿戴记录仓储。
func NewEquipRepo() *EquipRepo {
	return &EquipRepo{records: map[string]item.EquipRecord{}}
}

// keyOf 生成穿戴记录的复合存储键。
func keyOf(owner string, slot item.Slot, instanceID string) string {
	return owner + "|" + string(slot) + "|" + instanceID
}

// ListByOwner 列出玩家全部穿戴记录。
func (r *EquipRepo) ListByOwner(_ context.Context, owner string) ([]item.EquipRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []item.EquipRecord
	for _, rec := range r.records {
		if rec.Owner == owner {
			out = append(out, rec)
		}
	}
	return out, nil
}

// ListBySlot 列出玩家在指定槽位的穿戴记录（用于容量检查）。
func (r *EquipRepo) ListBySlot(_ context.Context, owner string, slot item.Slot) ([]item.EquipRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []item.EquipRecord
	for _, rec := range r.records {
		if rec.Owner == owner && rec.Slot == slot {
			out = append(out, rec)
		}
	}
	return out, nil
}

// Save 保存穿戴记录（同键覆盖）。
func (r *EquipRepo) Save(_ context.Context, rec item.EquipRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[keyOf(rec.Owner, rec.Slot, rec.InstanceID)] = rec
	return nil
}

// Delete 删除指定键的穿戴记录（幂等）。
func (r *EquipRepo) Delete(_ context.Context, owner string, slot item.Slot, instanceID string) error {
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

// RenameRecord 一次改名的记录。
type RenameRecord struct {
	Owner   string
	NewName string
}

// Renamer 改名服务端口的内存实现：只记录不改名，供测试断言。
type Renamer struct {
	mu      sync.Mutex
	records []RenameRecord
	// Current 改名后的当前昵称表。
	Current map[string]string
}

// NewRenamer 创建改名记录器。
func NewRenamer() *Renamer {
	return &Renamer{Current: map[string]string{}}
}

// Rename 记录一次改名并更新当前昵称。
func (r *Renamer) Rename(_ context.Context, owner, newName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, RenameRecord{Owner: owner, NewName: newName})
	r.Current[owner] = newName
	return nil
}

// Records 返回改名历史拷贝（测试断言入口）。
func (r *Renamer) Records() []RenameRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]RenameRecord(nil), r.records...)
}

// LevelSource 玩家等级源的内存实现，供座驾等级条件查询。
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
func (s *LevelSource) Level(_ context.Context, owner string) int64 {
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
		return &item.ErrConflict{Entity: "relation " + rel.ID}
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
		return &item.ErrVersionConflict{Entity: "relation " + rel.ID}
	}
	cp := *rel
	r.relations[rel.ID] = &cp
	return nil
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

// Stack 一键装配的完整内存栈：库存服务与全部基础设施组件。
// 组件字段开放访问，便于注册依赖端口的道具定义与测试断言
// （如 s.Levels.SetLevel、s.Defs.Register）。
type Stack struct {
	// Inv 通用库存服务。
	Inv *item.Inventory
	// Defs 道具定义注册表（可在装配后继续 Register）。
	Defs *DefRegistry
	// Bus 事件总线（含历史留痕）。
	Bus *EventBus
	// Ledger 货币账本。
	Ledger *Ledger
	// Instances 实例仓储。
	Instances *InstanceRepo
	// Equips 穿戴记录仓储。
	Equips *EquipRepo
	// Idempotency 幂等键存储。
	Idempotency *IdempotencyStore
	// Levels 等级源。
	Levels *LevelSource
	// Relations 关系仓储。
	Relations *RelationRepo
	// Banner 飘屏记录器。
	Banner *BannerRecorder
	// Renamer 改名记录器。
	Renamer *Renamer
}

// NewStack 装配内存栈。不带参数创建空栈，随后用 Defs.Register 注册道具定义
// （定义常持有 Levels/Banner 等组件，先建栈再注册是常规顺序）；
// 也可以直接传入不依赖内存组件的定义一步到位。
// 自定义时钟/随机源：装配后直接替换 Inv.Now / Inv.NewID。
func NewStack(defs ...item.Def) *Stack {
	bus := NewEventBus()
	defs2 := NewDefRegistry()
	defs2.Register(defs...)
	instances := NewInstanceRepo()
	equips := NewEquipRepo()
	idem := NewIdempotencyStore()
	ledger := NewLedger()
	levels := NewLevelSource()
	relations := NewRelationRepo()
	inv := item.New(item.Inventory{
		Defs:        defs2,
		Instances:   instances,
		Equips:      equips,
		Idempotency: idem,
		Events:      bus,
		Relations:   relations,
		NewID:       NewIDGenerator("inst_").Next,
	})
	return &Stack{
		Inv:         inv,
		Defs:        defs2,
		Bus:         bus,
		Ledger:      ledger,
		Instances:   instances,
		Equips:      equips,
		Idempotency: idem,
		Levels:      levels,
		Relations:   relations,
		Banner:      NewBannerRecorder(),
		Renamer:     NewRenamer(),
	}
}
