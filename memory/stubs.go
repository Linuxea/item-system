package memory

import (
	"context"
	"sync"
	"time"
)

// 本文件是几个外部服务的替身，用于演示与测试。
// 真实项目里它们会被换成 RPC 客户端，道具那边一行都不用改 ——
// 因为道具依赖的是 port 包里的接口，不是这些具体类型。

// Users 用户服务替身：把昵称记在内存里。
type Users struct {
	mu    sync.Mutex
	names map[string]string
}

// NewUsers 创建用户服务替身。
func NewUsers() *Users { return &Users{names: map[string]string{}} }

// Rename 修改昵称。
func (u *Users) Rename(_ context.Context, owner, newName string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.names[owner] = newName
	return nil
}

// NameOf 读取昵称。
func (u *Users) NameOf(owner string) string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.names[owner]
}

// Ledger 货币账本替身。
type Ledger struct {
	mu       sync.Mutex
	balances map[string]map[string]int64
}

// NewLedger 创建账本替身。
func NewLedger() *Ledger { return &Ledger{balances: map[string]map[string]int64{}} }

// Add 增加货币。
func (l *Ledger) Add(_ context.Context, owner, currency string, amount int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.balances[owner] == nil {
		l.balances[owner] = map[string]int64{}
	}
	l.balances[owner][currency] += amount
	return nil
}

// Balance 读取余额。
func (l *Ledger) Balance(owner, currency string) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.balances[owner][currency]
}

// Levels 等级来源替身。
type Levels struct {
	mu sync.Mutex
	lv map[string]int64
}

// NewLevels 创建等级替身。
func NewLevels() *Levels { return &Levels{lv: map[string]int64{}} }

// Set 设置玩家等级。
func (s *Levels) Set(owner string, level int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lv[owner] = level
}

// LevelOf 读取玩家等级。
func (s *Levels) LevelOf(_ context.Context, owner string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lv[owner], nil
}

// BannerMsg 一条已广播的飘屏。
type BannerMsg struct {
	// Owner 发起者。
	Owner string
	// Text 文案。
	Text string
	// Seconds 停留秒数。
	Seconds int
}

// Broadcaster 广播网关替身：把飘屏记在内存里。
type Broadcaster struct {
	mu   sync.Mutex
	sent []BannerMsg
}

// NewBroadcaster 创建广播替身。
func NewBroadcaster() *Broadcaster { return &Broadcaster{} }

// Broadcast 记录一条飘屏。
func (b *Broadcaster) Broadcast(_ context.Context, owner, text string, seconds int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent = append(b.sent, BannerMsg{Owner: owner, Text: text, Seconds: seconds})
	return nil
}

// Sent 返回已广播的飘屏。
func (b *Broadcaster) Sent() []BannerMsg {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]BannerMsg{}, b.sent...)
}

// Clock 可推进的时钟：测试里用它代替真实时间，不必 sleep。
type Clock struct {
	mu sync.Mutex
	t  time.Time
}

// NewClock 创建时钟。
func NewClock(t time.Time) *Clock { return &Clock{t: t} }

// Now 返回当前时刻。
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Advance 把时钟向前推进。
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}
