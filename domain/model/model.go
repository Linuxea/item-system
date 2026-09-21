package model

import "time"

type Category string

const (
	CategoryAvatar     Category = "avatar"
	CategoryNamePlate  Category = "nameplate"
	CategoryBadge      Category = "badge"
	CategoryVIP        Category = "vip"
	CategoryConsumable Category = "consumable"
	CategoryCurrency   Category = "currency"
	CategoryMount      Category = "mount"
)

type SlotType string

const (
	SlotAvatar    SlotType = "avatar"
	SlotNamePlate SlotType = "nameplate"
	SlotBadge     SlotType = "badge"
	SlotVIP       SlotType = "vip"
	SlotMount     SlotType = "mount"
)

type InstanceStatus string

const (
	StatusNormal   InstanceStatus = "normal"
	StatusEquipped InstanceStatus = "equipped"
	StatusExpired  InstanceStatus = "expired"
)

type Modifier struct {
	Key   string
	Value any
}

type ItemTemplate struct {
	ID        string
	Category  Category
	Name      string
	Priority  int
	Rarity    int
	Behaviors map[string]map[string]any
	Config    map[string]any
}

type ItemInstance struct {
	ID         string
	TemplateID string
	Owner      string
	Count      int64
	Bound      bool
	Status     InstanceStatus
	AcquiredAt time.Time
	ExpireAt   *time.Time
	Version    int64
}

func (i *ItemInstance) Expired(now time.Time) bool {
	return i.ExpireAt != nil && !now.Before(*i.ExpireAt)
}

func (i *ItemInstance) Available(now time.Time) bool {
	return i.Status == StatusNormal && !i.Expired(now)
}

func (i *ItemInstance) BumpVersion() int64 {
	i.Version++
	return i.Version
}

type EquipRecord struct {
	Owner      string
	Slot       SlotType
	InstanceID string
	EquippedAt time.Time
}

type GrantResult struct {
	InstanceID string
	Count      int64
}

type UseResult struct {
	InstanceID string
	Consumed   int64
}
