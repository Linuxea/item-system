package repository

import (
	"context"
	"time"

	"github.com/linuxea/item-system/domain/model"
)

type TemplateSource interface {
	Get(ctx context.Context, id string) (*model.ItemTemplate, error)
}

type InstanceRepo interface {
	Save(ctx context.Context, inst *model.ItemInstance) error
	Get(ctx context.Context, id string) (*model.ItemInstance, error)
	ListByOwner(ctx context.Context, owner string) ([]*model.ItemInstance, error)
	Update(ctx context.Context, inst *model.ItemInstance, expectVersion int64) error
	Delete(ctx context.Context, id string) error
	ListExpired(ctx context.Context, before time.Time, limit int) ([]*model.ItemInstance, error)
}

type EquipRepo interface {
	ListByOwner(ctx context.Context, owner string) ([]model.EquipRecord, error)
	ListBySlot(ctx context.Context, owner string, slot model.SlotType) ([]model.EquipRecord, error)
	Save(ctx context.Context, rec model.EquipRecord) error
	Delete(ctx context.Context, owner string, slot model.SlotType, instanceID string) error
	DeleteByInstance(ctx context.Context, instanceID string) error
}

type IdempotencyStore interface {
	Claim(ctx context.Context, key string) (bool, error)
	Release(ctx context.Context, key string) error
}

type ErrNotFound struct{ Entity string }

func (e *ErrNotFound) Error() string { return e.Entity + " not found" }

type ErrVersionConflict struct{ Entity string }

func (e *ErrVersionConflict) Error() string { return e.Entity + " version conflict" }
