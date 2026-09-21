package event

import "time"

const (
	ItemGranted    = "item.granted"
	ItemConsumed   = "item.consumed"
	ItemEquipped   = "item.equipped"
	ItemUnequipped = "item.unequipped"
	ItemExpired    = "item.expired"
)

type Event interface {
	Name() string
	OccurredAt() time.Time
}

type Base struct {
	At time.Time
}

func (b Base) OccurredAt() time.Time { return b.At }

type Granted struct {
	Base
	Owner      string
	TemplateID string
	InstanceID string
	Count      int64
	Source     string
	Reason     string
}

func (e Granted) Name() string { return ItemGranted }

type Consumed struct {
	Base
	Owner      string
	TemplateID string
	InstanceID string
	Count      int64
}

func (e Consumed) Name() string { return ItemConsumed }

type Equipped struct {
	Base
	Owner      string
	TemplateID string
	InstanceID string
	Slot       string
}

func (e Equipped) Name() string { return ItemEquipped }

type Unequipped struct {
	Base
	Owner      string
	TemplateID string
	InstanceID string
	Slot       string
}

func (e Unequipped) Name() string { return ItemUnequipped }

type Expired struct {
	Base
	Owner      string
	TemplateID string
	InstanceID string
	Policy     string
}

func (e Expired) Name() string { return ItemExpired }

type Publisher interface {
	Publish(events ...Event)
}
