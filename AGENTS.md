# AGENTS.md

Go library (`github.com/Linuxea/item-system`). No CI, no Makefile — plain Go toolchain.
Read `docs/DESIGN.md` before changing anything structural.

## Verify before committing

```sh
gofmt -w . && go vet ./... && go test -race ./...
```

Race mode matters: repos and the event bus are concurrently accessed.

## Core rule

An item's capabilities are the interfaces it implements (`item/item.go`). No config maps,
no registry of behavior strings, no reflection. The engine branches only via type assertion.

## Hard rules (review blockers)

- **`engine/` must not name a concrete item.** Verify with
  `go list -deps ./engine | grep item-system/items` — it must print nothing.
  (Grepping for `items.` gives false positives: `e.items` is the registry field.)
- **Dependency direction**: `item` and `port` import nothing of ours. `items → item + port`.
  `engine → item + event`. `relation → item + event`. `memory` is the only package that
  imports everything; it is the composition root.
- **Chinese comments required**: every exported identifier gets a godoc comment starting
  with the identifier name. Key invariants (the deduct-before-execute ordering, CAS,
  the Bind copy, the three wiring cycles) get inline comments. No commented-out code.
- **Parameters are struct fields, never a map crossing an interface.** `Use(ctx, owner)`
  takes no params. User input binds in the item's own `Bind`, which copies the prototype.
  Never mutate a prototype.
- **Validation happens in `Bind`, before any side effect.** A failed `Bind` must leave the
  player's inventory untouched.
- **Use deducts before executing.** Do not "simplify" this to execute-then-deduct; it
  reopens a concurrency hole. See `TestConcurrentUseDeductsExactlyOnce`.
- **New item = new struct in `items/` + a line in `Catalog` + its row in
  `items/capabilities.go`.** Never a change to `engine/`.
- **New capability = an interface in `item/item.go` + one type assertion in `engine/`.**
  Nothing else.
- `engine.LateGranter` and `relation.Service.BindUnequipper` exist to break wiring cycles.
  Do not remove them.

## Testing conventions

- Tests that need the wired stack live in `package memory_test` (external test package).
- Deterministic time: `memory.NewStack(memory.StackOptions{Now: epoch})`, then
  `s.Clock.Advance(d)`. Never sleep.
- Deterministic RNG: pass `Rand: func(int64) int64 { return 0 }` in `StackOptions`.
- Every new capability needs a test proving items *without* it are rejected
  (see `TestUseNotUsableItem`, `TestEquipNotEquippable`).

## Git

- Commit message: one imperative English sentence, no trailing period.
- One logical unit per commit.
