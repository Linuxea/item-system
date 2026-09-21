// Package event 提供领域事件的公共词汇：事件接口与发布端口。
// item 与 relation 两个聚合共用此抽象，事件总线等实现方依赖这里。
package event

import "time"

// Event 领域事件接口：事件名 + 发生时间。
type Event interface {
	Name() string
	OccurredAt() time.Time
}

// Base 事件公共部分，内嵌到具体事件以获得 OccurredAt 实现。
type Base struct {
	// At 事件发生时间。
	At time.Time
}

// OccurredAt 返回事件发生时间。
func (b Base) OccurredAt() time.Time { return b.At }

// Publisher 领域事件发布端口；实现方负责同步或异步分发。
type Publisher interface {
	Publish(events ...Event)
}
