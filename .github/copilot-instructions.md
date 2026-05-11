# GitHub Copilot Instructions — aisync

This file gives GitHub Copilot (and other AI assistants that read repository
instructions) the context needed to produce code that matches the project's
architecture, testing discipline, and security posture.

## Project Summary

**aisync** is a Go CLI that synchronizes AI coding assistant configurations
(rules, agents, commands, hooks, skills, memories, settings) across multiple
devices and 30+ AI tools. It pulls shared rules from public sources via Git
tarball downloads, syncs personal configurations through a private Git repo,
and encrypts sensitive data with `age`.

The canonical, human-written deep-dive is [`CLAUDE.md`](../CLAUDE.md). When
this file and `CLAUDE.md` disagree, `CLAUDE.md` wins — update both together.

## Build, Test, and Lint — Always Use the Makefile

```bash
make build          # Compile to bin/aisync (stripped with -s -w)
make debug          # Compile with debug symbols (-gcflags "-N -l")
make run            # go run ./cmd/aisync
make install        # Build and copy to ~/.local/bin/aisync
make lint           # golangci-lint via pipelines repo config
make test           # Full unit test suite (~2 seconds)
make sast           # CodeQL, Semgrep, Trivy, Hadolint, Gitleaks
```

**Never call tool binaries directly** (`golangci-lint`, `semgrep`, `trivy`,
`hadolint`, `gitleaks`, `go test` without the unit tag, etc.). The Makefile
imports shared targets from the [`pipelines`](https://github.com/rios0rios0/pipelines)
repo that load the correct configuration before invoking each tool. Calling
binaries directly bypasses that configuration and produces false positives.

Running a single test during development is the only exception:

```bash
go test -tags unit -run "TestHooksMerger_Merge" ./internal/infrastructure/services/
```

All unit test files carry `//go:build unit`. Build and the unit suite each
complete in under three seconds, so run them on every change — don't save
them for the end.

## Architecture — Clean / Hexagonal

Dependencies always point inward. Infrastructure depends on domain
interfaces; the domain layer never imports anything from infrastructure, and
entities never import frameworks or tags.

```
cmd/aisync/                      Entry point. Sets up logrus, calls
                                 controllers.NewRootCommand(version).

internal/domain/                 (contracts — zero framework dependencies)
  commands/                        use cases with a single Execute() method
  entities/                        value objects / aggregates
  repositories/                    repository INTERFACES (ports)

internal/infrastructure/         (implementations — adapters)
  controllers/                     cobra wiring + manual DI composition root
  repositories/                    YAML / JSON / HTTP / Git persistence
  services/                        encryption, mergers, watchers, scanners
  ui/                              lipgloss formatter

test/doubles/                    Manual struct-based stubs for all domain
                                 interfaces. No mocking framework.
```

### Dependency Injection (Manual)

`internal/infrastructure/controllers/root.go` is the composition root. It is
the **only** place where concrete infrastructure types are referenced.
`NewRootCommand(version)` builds every repository, service, and domain
command, then registers each cobra subcommand. Domain command constructors
accept interfaces only — never a concrete infrastructure type.

When adding a new command, touch exactly these four locations:

1. `internal/domain/commands/<name>.go` — struct with a constructor that
   accepts domain interfaces and a single `Execute(...)` method.
2. `internal/infrastructure/controllers/root.go` — instantiate the command,
   write a `new<Name>Subcmd()` helper that wraps it in a `cobra.Command`, add
   it to `root.AddCommand(...)`.
3. `test/doubles/mocks.go` — add stubs for any new domain interfaces.
4. `internal/domain/commands/<name>_test.go` — unit tests using the stubs.

Do not introduce a DI framework (Uber Dig, Wire, etc.). Manual constructor
wiring is intentional — it keeps the composition root auditable and the
domain layer framework-free.

### Bundle Sync

Per-tool project bundles (`tools.<name>.bundles[]` in `config.yaml`) sync opaque
directory trees as age-encrypted tarballs so directory names never appear in the
git tree. Two modes: `subdirs` (default, one tarball per immediate subdirectory,
filename is `hmac_sha256(per_repo_key, name)[:16].age` where the per-repo key is
HKDF-derived from the device's age identity) and `whole` (entire source directory
as one tarball). Two merge strategies on pull: `mtime` (default, newer-wins with
local-only preservation) and `replace` (overwrite unconditionally). Cross-device
deletion detection uses `~/.cache/aisync/bundle-state.json` and prompts before
removing. See `CLAUDE.md` § Bundle Sync for full details.

## Go Conventions

- **File names:** `snake_case` (`list_users_command.go`, not `ListUsers.go`).
- **Receivers:** short abbreviation of the type (`c` for `Command`, `r` for
  `Repository`). Consistent across all methods of the same type. Never
  `self`, `this`, or `me`.
- **Logging:** import Logrus as `logger "github.com/sirupsen/logrus"`.
  Always use structured logging (`logger.WithFields(...)`) instead of
  `fmt.Sprintf`-style interpolation. Never use the stdlib `log` package or
  `fmt.Println` for application logging.
- **Accept interfaces, return structs.** Functions take domain interfaces so
  the caller is flexible, and return concrete types so the implementation is
  explicit.
- **Small interfaces.** "The bigger the interface, the weaker the
  abstraction." If a function only reads items, accept a single-method
  reader interface instead of the full repository.
- **No `any` in public signatures.** Use generics with type constraints or
  a proper interface instead.
- **Entities are framework-agnostic.** No `json:"..."` or other struct tags
  inside `internal/domain/entities/`. Tags belong only on DTOs in the
  infrastructure layer.

## Testing Discipline

Tests in this repo follow a strict BDD pattern — Copilot's suggestions must
match this pattern, not generate free-form tests.

- **Build tag:** every test file starts with `//go:build unit`.
- **External test package:** if production code is in `package commands`,
  the test file uses `package commands_test` and accesses only exported
  API. Never reach into internals.
- **BDD blocks:** every test body has three comment-delimited blocks,
  `// given` / `// when` / `// then`. Preconditions, action, assertions.
- **Test name shape:** `TestTypeName_MethodName_DescriptiveBehavior`
  (e.g. `TestHooksMerger_Merge_ConcatenatesArrays`), grouping each scenario
  under `t.Run("should ... when ...", func(t *testing.T) { ... })`.
- **Parallelism:** unit tests call `t.Parallel()` at the top of the parent
  `Test...` function so all sub-tests run concurrently.
- **Assertions:** `testify/assert` for assertions, `testify/require` for
  fatal preconditions (stop the test if they fail).
- **Doubles:** use the manual stubs in `test/doubles/mocks.go`. They store
  call counts and captured arguments. Do not introduce a mocking framework
  (gomock, mockery, testify/mock, etc.). If a new domain interface is added,
  extend `mocks.go` with a struct-based stub in the same style.

Test description patterns by layer:

| Layer            | Format                                          |
|------------------|-------------------------------------------------|
| Command          | `"should call <listener> when ..."`             |
| Controller       | `"should respond <STATUS> when ..."`            |
| Service / Repo   | `"should ... when ..."` (success + failure)    |

## Security Rules (Non-Negotiable)

- **Never hard-code secrets.** No API keys, tokens, passwords, or private
  keys in source. Use environment variables or secret managers.
- **Secret scanning is enforced by `make sast`.** If Gitleaks flags a
  match, rotate the credential and remove it from history before pushing.
- **The encrypt path has two independent gates.** A file is written as
  ciphertext only when BOTH `encryptPatterns.Matches(...)` and
  `len(config.Encryption.Recipients) > 0` are true. Both content scanners
  (secret + NDA) must mirror that gate so files written as plaintext
  (empty recipients) are still scanned. See `PushCommand.copyPersonalFile`
  and `PushCommand.collectUnencryptedFiles` /
  `PushCommand.runSecretScan` / `PushCommand.runNDAScan` in
  `internal/domain/commands/push.go`.
- **Four-layer push protection stack.** Every push runs four orthogonal
  gates that each catch a different leak class. They compose, they do not
  overlap. **Never delete or weaken any layer without an explicit
  user-facing decision in the PR description.**
  1. **Per-tool allowlist** (`internal/domain/entities/allowlist.go`) —
     unknown content is never synced; users opt in via
     `tools.<name>.extra_allowlist`. The strict matcher does NOT fall back
     to basename matching.
  2. **`.aisyncignore`** — gitignore-syntax additional exclusions.
  3. **`.aisyncencrypt` + age** — paths matching the patterns are
     written as ciphertext to the configured recipients.
  4. **Content scanners** —
     - `RegexSecretScanner` (15 credential-format regexes), bypass
       `--skip-secret-scan`.
     - `CompositeNDAChecker` (explicit list at
       `<repo>/.aisync-forbidden.age` + auto-derived from machine state +
       compile-time heuristic shape checks), bypass `--skip-nda-scan`.
     Findings are tagged with the source (`user`,
     `auto-derived:<origin>`, `heuristic:<name>`) so the user knows
     which knob fixes each hit.
- **NDA scanner stays in the dry-run path.** `executeDryRun` runs the
  same secret + NDA scanners against the would-be-pushed file contents
  that `commitAndPush` runs in the real-push path. `--dry-run` must
  preview the actual blocks, not just enumerate paths — that is the
  whole point of `--dry-run`.
- **Domain layer never imports infrastructure.** `PushCommand` depends
  on `repositories.NDAContentChecker` (a single-method facade defined in
  the domain layer); the composite implementation lives in
  `infrastructure/services/nda_content_checker.go` and is wired in
  `controllers/root.go:buildNDAStack`. `NDACommand` similarly takes the
  heuristic count as an `int` constructor parameter rather than calling
  `services.HeuristicCount()` directly.
- **Path-matching consistency.** Every `.aisyncencrypt` match site must
  build the repo-relative path via `encryptMatchPath(toolName, relPath)` or
  reuse an already repo-relative `relPath` — never hand-roll a
  `filepath.Join("personal", ...)`. Drift between match sites has
  previously caused silent plaintext commits.

## Documentation Discipline

Every code change ships with documentation in the same commit/PR:

- **`CHANGELOG.md`** — always. Add an entry under `[Unreleased]` using one
  of the Keep-a-Changelog categories (`Added`, `Changed`, `Deprecated`,
  `Removed`, `Fixed`, `Security`). Simple past tense, lowercase first verb,
  no trailing period.
- **`README.md`** — when usage, CLI flags, or setup changes.
- **`CLAUDE.md`** — when architecture, build commands, or development
  workflow changes.
- **This file (`.github/copilot-instructions.md`)** — when the workflow
  that Copilot should reinforce changes.

Do not let a PR land with behavior changes and no changelog entry.

## Commit and Branch Conventions

- **Branches:** `feat/<slug>`, `fix/<slug>`, `chore/<slug>`, `refactor/...`,
  `test/...`, `docs/...`. Include a ticket ID when one exists.
- **Commit format:** `type(scope): message` — simple past tense, lowercase
  first word, no trailing period. Example:
  `fix(push): gated secret scan on recipients`.
- **Flag breaking changes** in three places: the commit footer
  (`**BREAKING CHANGE:** ...`), `CHANGELOG.md`, and the PR description.
- **Synchronize branches with rebase, not merge.** Rewriting history keeps
  `main` linear.

## Anti-Patterns to Avoid

Copilot should actively steer away from these:

- Running `golangci-lint`, `semgrep`, `trivy`, or other tool binaries
  directly instead of `make lint` / `make sast`.
- Using `interface{}` / `any` as a catch-all parameter.
- Writing tests without `// given` / `// when` / `// then` blocks.
- Writing tests in the same package as production code
  (`package commands`) instead of `package commands_test`.
- Introducing a mocking framework. Extend `test/doubles/mocks.go` instead.
- Adding framework tags (`json:"..."`, `gorm:"..."`) to structs in
  `internal/domain/entities/`.
- Importing anything from `internal/infrastructure/` inside
  `internal/domain/`.
- Using the standard library `log` package or `fmt.Println` for
  application logging. Use `logger` (Logrus).
- Hand-rolling encrypt-match paths instead of calling
  `encryptMatchPath(toolName, relPath)`.
- Importing `internal/infrastructure/services` from
  `internal/domain/commands/nda.go` (or any domain command) to read
  `services.HeuristicCount()`. The count must travel through the
  constructor as an `int`. The same rule applies to any future scanner
  configuration the domain layer needs to know about.
- Skipping the secret or NDA scanner in `executeDryRun` to keep dry-run
  "fast." Dry-run must run the same content scanners as the real-push
  path so users discover blocks before they commit.
- Removing or weakening any of the four push-protection layers (per-tool
  allowlist, `.aisyncignore`, `.aisyncencrypt`, content scanners) without
  an explicit user-facing decision in the PR description.
- Storing the explicit forbidden-terms list anywhere except encrypted
  inside the sync repo at `<repo>/.aisync-forbidden.age`. Plaintext on
  disk would defeat the whole point of the scanner.
- Hard-coding secrets of any kind.
- Landing a code change without updating `CHANGELOG.md`.

## Where to Look First

When asked to change something, read these files before suggesting edits:

| Topic                                          | Start here                                                                |
|------------------------------------------------|---------------------------------------------------------------------------|
| Adding / changing a command                    | `internal/infrastructure/controllers/root.go`                             |
| Push pipeline (copy, encrypt, scan)            | `internal/domain/commands/push.go`                                        |
| Pull pipeline (fetch, merge, apply)            | `internal/domain/commands/pull.go`                                        |
| Encrypt / ignore pattern matching              | `internal/domain/entities/{encrypt,ignore}_patterns.go`                   |
| Per-tool allowlist (replaces old deny-list)    | `internal/domain/entities/allowlist.go`                                   |
| Credential regex scanner                       | `internal/infrastructure/services/secret_scanner.go`                      |
| NDA scanner (3 sources + heuristics)           | `internal/infrastructure/services/{nda_scanner,auto_deriver,nda_content_checker}.go` |
| NDA forbidden-terms entity (canonical match)   | `internal/domain/entities/forbidden_terms.go`                             |
| Encrypted forbidden-terms repo                 | `internal/infrastructure/repositories/age_forbidden_terms_repository.go`  |
| `aisync nda` command (add/remove/list/ignore)  | `internal/domain/commands/nda.go`                                         |
| File merge strategies                          | `internal/infrastructure/services/{hooks,settings,section}_merger.go`     |
| Atomic apply (two-phase commit)                | `internal/infrastructure/services/atomic_apply_service.go`                |
| Bundle sync (tar+age packaging)                | `internal/infrastructure/services/tar_age_bundle_service.go`              |
| Bundle push / pull integration                 | `internal/domain/commands/{push,pull}_bundles.go`                         |
| Bundle prune (`aisync bundles prune`)          | `internal/domain/commands/bundles_prune.go`                               |
| Bundle entities + config (BundleSpec, modes)   | `internal/domain/entities/{bundle_manifest,bundle_state,tool}.go`         |
| Test stubs                                     | `test/doubles/mocks.go`                                                   |
