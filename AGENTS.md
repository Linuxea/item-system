# AGENTS.md

Go library (`github.com/linuxea/item-system`), Go 1.27. No CI, no Makefile — plain Go toolchain. Read `docs/DESIGN.md` first; it documents the architecture and the rationale behind every rule below.

## Verify before committing

```sh
gofmt -w . && go vet ./... && go test -race ./...
```

Race mode matters: repos and the event bus are concurrently accessed. A plain `go test` pass is not sufficient.

## Hard design rules (violations are review-blockers)

- **Domain purity**: `domain/` production code must not import `application`, `infrastructure`, or `catalog`. Tests may (via external test packages, see below).
- **Chinese comments required**: every exported identifier gets a godoc comment (中文, starting with the identifier name); key logic (materialization seam, narrow interfaces, CAS versioning) gets inline comments. No commented-out code.
- **Consumer-side narrow interfaces**: each service declares its own unexported interface for the store methods it uses (e.g. `instanceStore` in `domain/grant`). Never pass the full `repository.InstanceRepo` into a domain service. `application.Deps` may use full provider interfaces — it is the composition root.
- **Each command owns exactly its own deps**: `effect.Command` structs hold one private port (`BroadcastBanner.banner`, `Condition.checker`), injected via constructor (`NewBroadcastBanner`), called only in `Exec(ctx, owner)`. No god-bundle (no `Runtime`-style struct) may cross a runtime seam; `behavior.Ports` / `application.Deps` exist only at startup wiring.
- **Commands are complete before execution**: user input binds in `effect.Materialize` (the only place that sees `Params`); missing params fail with zero side effects. Never add params to `Execute`/`Exec` signatures.
- **New item types = data + composition**, never core changes: combine existing behaviors in `catalog/`, add a slot constant in `domain/model` if needed.
- **New effect = new Command struct** (private port + constructor + `Exec`) + a case in `behavior.decodeEffect` + a field in `behavior.Ports` if a new port is required. The central switch-free principle: no application-layer type switch on commands.
- `grantAdapter` in `application` exists solely to break the `GrantItem → grant.Service → Registry` cycle. Do not "simplify" it away.

## Testing conventions

- Domain tests that need the memory stack **must** be external packages (`package grant_test`); internal test files would create import cycles through `infrastructure/memory → application → domain`.
- Deterministic time: capture `now := time.Now(); clock := &now`, pass `Now: func() { return *clock }`, then advance `*clock` — see `newStackWithClock` in `application/mount_test.go`.
- `memory.RelationRepo.UseClock(fn)` is required for relation expiry tests (its `FindActive` filters by time internally).
- `forceExpire` (application tests) rewrites an instance's `ExpireAt` via versioned repo update — use it instead of sleeping.
- Fixed-RNG banner/chest tests: inject `Rand` through `application.Deps`; `fixedRand` in `application/application_test.go`.

## Wiring

`infrastructure/memory.NewStack(tpls...)` returns a fully wired stack (App + repos + bus + ledger + recorder + levels). For custom clocks/RNG build `application.Deps` manually (copy the pattern in `application/mount_test.go:16`).

## Git

- Commit message: one imperative English sentence, no trailing period (e.g. `Add mount item support with equip conditions and mount slot`).
- One logical unit per commit; user expects a commit per delivered feature/fix.
- Releases are annotated tags (`v1.0.0`).
