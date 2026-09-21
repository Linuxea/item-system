# AGENTS.md

Go library (`github.com/Linuxea/item-system`), Go 1.27. No CI, no Makefile — plain Go toolchain. Read `docs/DESIGN.md` first; it documents the architecture and the rationale behind every rule below.

## Verify before committing

```sh
gofmt -w . && go vet ./... && go test -race ./...
```

Race mode matters: repos and the event bus are concurrently accessed. A plain `go test` pass is not sufficient.

## Hard design rules (violations are review-blockers)

- **显式类型，不是数据驱动**：每种道具是一个 Go 类型（行为 = 方法，标签内嵌 `item.Common`）。禁止重新引入 v1 式的配置表/行为组件/效果命令层（见 git 历史 tag `v1.0.0`）——道具定义不需要运行时热更新，这是产品层面的既定决策。
- **新道具 = 新类型 + 能力接口**：`type X struct { item.Common; ...依赖 }`，实现 `item.Usable/Equippable/Expirable/Passive/EquipGated/RelationGated` 中想要的，不实现的能力通用层断言自然跳过。禁止给能力接口加配置开关。
- **道具自持业务逻辑**：改名卡 `Use` 里直接调用户服务、座驾 `CanEquip` 里直接查等级源。外部服务是道具类型的字段，由装配方构造注入。需要用户参数的道具必须在产生副作用前校验并返回 `item.ErrMissingParam`。
- **通用层只做种类无关的事**：`item.Inventory`（Grant/Use/Equip/RunExpiry/关系流程）不出现任何道具种类的 if/switch；能力分发只经 `def.(Usable)` 等断言。道具再发放用 `inv.Grant`，不要为此造新通道。
- **分层**：`item` 与 `relation` 不 import `memory`/`item/catalog`；`memory` 实现核心端口；测试用外部包 `item_test`（经 memory/catalog 间接引用，避免循环导入）。
- **乐观锁**：实例/关系写前必须 `BumpVersion`，仓储 `Update` 携带读取时的期望版本，冲突返回 `ErrVersionConflict`。
- **使用成功才扣减**：`Inventory.Use` 逐单位调用道具 `Use`，任一失败即中止且不扣减；业务失败的幂等键要释放。
- **Chinese comments required**: every exported identifier gets a godoc comment (中文, starting with the identifier name); key logic (stacking merge windows, downgrade inheriting target lifetime, dissolve linkage) gets inline comments. No commented-out code.

## Testing conventions

- Tests live in `item/` as external package `item_test`.
- Deterministic time: after `memory.NewStack()`, replace `stack.Inv.Now = func() time.Time { return *clock }`, then advance `*clock` — see `newStack` in `item/helpers_test.go`.
- `memory.RelationRepo.UseClock(fn)` is required for relation expiry tests (its `FindActive` filters by time internally).
- `forceExpire` (item tests) rewrites an instance's `ExpireAt` via versioned repo update — use it instead of sleeping.
- Fixed-RNG chest tests: inject `fixedRand` into `catalog.Chests(rnd.next)`.

## Wiring

`memory.NewStack(defs ...item.Def)` returns a fully wired stack (Inv + Defs registry + repos + bus + ledger + levels + relations + banner + renamer). Item defs that need stack components (levels/banner/ledger/rand/rename service) are registered after stack creation: `stack.Defs.Register(catalog.Mounts(stack.Levels)...)`.

## Git

- Commit message: one imperative English sentence, no trailing period (e.g. `Redesign items as explicit Go types with an inventory core`).
- One logical unit per commit; user expects a commit per delivered feature/fix.
- Releases are annotated tags (`v1.0.0` = 数据驱动旧架构；`v2.0.0` = 显式类型重设计).
