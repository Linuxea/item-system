// Package repository 声明道具领域持久化依赖的提供方接口（端口）。
// 领域只依赖此处的抽象，由基础设施提供实现（依赖倒置）。
// 各领域服务还会在自己的包内声明更窄的消费侧接口，只使用本包接口的一个切片，
// 因此本包接口是最全集，运行期实现必须完整满足它。
package repository

import (
	"context"
	"time"

	"github.com/Linuxea/item-system/domain/model"
)

// TemplateSource 模板只读源。模板是只读配置，无写入端口。
type TemplateSource interface {
	Get(ctx context.Context, id string) (*model.ItemTemplate, error)
}

// InstanceRepo 道具实例仓储的提供方全集接口。
// Update 采用乐观锁：expectVersion 必须等于读取时的版本号，否则返回 ErrVersionConflict。
type InstanceRepo interface {
	Save(ctx context.Context, inst *model.ItemInstance) error
	Get(ctx context.Context, id string) (*model.ItemInstance, error)
	ListByOwner(ctx context.Context, owner string) ([]*model.ItemInstance, error)
	Update(ctx context.Context, inst *model.ItemInstance, expectVersion int64) error
	Delete(ctx context.Context, id string) error
	ListExpired(ctx context.Context, before time.Time, limit int) ([]*model.ItemInstance, error)
}

// EquipRepo 穿戴记录仓储的提供方全集接口。
type EquipRepo interface {
	ListByOwner(ctx context.Context, owner string) ([]model.EquipRecord, error)
	ListBySlot(ctx context.Context, owner string, slot model.SlotType) ([]model.EquipRecord, error)
	Save(ctx context.Context, rec model.EquipRecord) error
	Delete(ctx context.Context, owner string, slot model.SlotType, instanceID string) error
	DeleteByInstance(ctx context.Context, instanceID string) error
}

// IdempotencyStore 幂等键存储。
// Claim 抢占键（false 表示已被占用），Release 在业务失败回滚时释放键。
type IdempotencyStore interface {
	Claim(ctx context.Context, key string) (bool, error)
	Release(ctx context.Context, key string) error
}

// ErrNotFound 实体不存在错误，Entity 描述缺失的实体。
type ErrNotFound struct{ Entity string }

func (e *ErrNotFound) Error() string { return e.Entity + " not found" }

// ErrConflict 实体已存在（唯一性冲突）错误。
type ErrConflict struct{ Entity string }

func (e *ErrConflict) Error() string { return e.Entity + " already exists" }

// ErrVersionConflict 乐观锁版本冲突错误：写入时的期望版本与存储中的当前版本不一致。
type ErrVersionConflict struct{ Entity string }

func (e *ErrVersionConflict) Error() string { return e.Entity + " version conflict" }
