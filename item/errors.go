package item

import "errors"

// 领域错误。调用方用 errors.Is 判别，不要比对字符串。
var (
	// ErrUnknownItem 道具 ID 未登记。
	ErrUnknownItem = errors.New("item: 未知道具")
	// ErrNoSuchInstance 实例不存在，或不属于该玩家。
	ErrNoSuchInstance = errors.New("item: 实例不存在或不属于该玩家")
	// ErrNotUsable 该道具不可主动使用。
	ErrNotUsable = errors.New("item: 此道具不可使用")
	// ErrNotEquippable 该道具不可穿戴。
	ErrNotEquippable = errors.New("item: 此道具不可穿戴")
	// ErrNotEquipped 该实例当前并未穿戴。
	ErrNotEquipped = errors.New("item: 此道具未在穿戴中")
	// ErrNotEnough 数量不足。
	ErrNotEnough = errors.New("item: 数量不足")
	// ErrSlotFull 槽位已满。
	ErrSlotFull = errors.New("item: 槽位已满")
	// ErrExpired 实例已过期。
	ErrExpired = errors.New("item: 道具已过期")
	// ErrMissingParam 使用时缺少必需参数。
	ErrMissingParam = errors.New("item: 缺少必需参数")
	// ErrInvalidParam 使用时参数不合法。
	ErrInvalidParam = errors.New("item: 参数不合法")
	// ErrConditionNotMet 未满足穿戴前置条件。
	ErrConditionNotMet = errors.New("item: 未满足穿戴条件")
	// ErrVersionConflict 乐观锁版本冲突，需重试。
	ErrVersionConflict = errors.New("item: 并发冲突，请重试")
	// ErrDuplicateRequest 幂等键重复，本次请求已被处理过。
	ErrDuplicateRequest = errors.New("item: 请求重复")
)
