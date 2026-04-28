# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Project Does

aisync is a Go CLI tool that synchronizes AI coding assistant configurations (rules, agents, commands, hooks, skills, memories, settings) across multiple devices and 30+ AI tools. It pulls shared rules from public external sources via Git tarball downloads, syncs personal configurations via a private Git repository, and encrypts sensitive data with age.

## Build & Development Commands

```bash
make build          # Compile to bin/aisync (stripped with -s -w)
make debug          # Compile with debug symbols (-gcflags "-N -l")
make run            # go run ./cmd/aisync
make install        # Build and copy to ~/.local/bin/aisync
make lint           # golangci-lint (always use this, never call golangci-lint directly)
make test           # Full test suite via pipeline scripts
make sast           # Security analysis (CodeQL, Semgrep, Trivy, Hadolint, Gitleaks)
```

Run a single test during development:
```bash
go test -tags unit -run "TestHooksMerger_Merge" ./internal/infrastructure/services/
```

All unit test files require the `//go:build unit` build tag. Build and test suite each run in ~2 seconds.

## Architecture

Clean Architecture with Hexagonal (Ports & Adapters) design. Dependencies always point inward: infrastructure depends on domain interfaces, never the reverse. No DI framework — manual constructor wiring in `controllers/root.go`.

### Layer Boundaries

- **`cmd/aisync/`** — Entry point. Sets up logrus, creates the cobra root command via `controllers.NewRootCommand(version)`, and calls `Execute()`.
- **`internal/domain/`** — Zero framework dependencies. Contains commands (use cases), entities (value objects/aggregates), and repository interfaces (ports).
- **`internal/infrastructure/`** — Implements domain interfaces. Contains `controllers/` (cobra wiring + DI), `repositories/` (YAML/JSON/HTTP/Git persistence), `services/` (encryption, merging, watching, scanning), and `ui/` (lipgloss formatter).
- **`test/doubles/`** — Manual struct-based stubs for all domain interfaces. Each stub stores call counts and captured args for assertion — no mocking framework.

### How Dependency Injection Works

`controllers/root.go` is the composition root. `NewRootCommand(version)` creates every infrastructure instance, injects them into domain command constructors, and wires cobra subcommands. This is the only place where concrete types are referenced — domain commands accept only interfaces.

```
main.go → NewRootCommand(version)
             ├── creates repos: YAMLConfigRepo, HTTPSourceRepo, JSONManifestRepo, JSONStateRepo, GoGitRepo, JSONJournalRepo, JSONBundleStateRepo
             ├── creates services: AgeEncryptionSvc, HooksMerger, SettingsMerger, SectionMerger, AtomicApplySvc, WatchSvc, TarAgeBundleSvc, ...
             ├── creates domain commands (injecting interface deps)
             └── registers cobra subcommands → returns root *cobra.Command
```

### Adding a New Command

Adding a command touches 4 locations:

1. **`internal/domain/commands/`** — Create `foo.go` with a `FooCommand` struct. Constructor accepts domain interfaces. Single `Execute()` method with business logic.
2. **`internal/infrastructure/controllers/root.go`** — Instantiate `FooCommand` with infrastructure deps, create a `newFooSubcmd()` helper that wraps it in a `cobra.Command`, add to `root.AddCommand(...)`.
3. **`test/doubles/mocks.go`** — Add stubs for any new domain interfaces (if introduced).
4. **`internal/domain/commands/foo_test.go`** — Unit tests using stubs from `test/doubles/`.

### Merger Polymorphism

The `Merger` interface has three implementations, each handling a different single-file merge strategy:

| Implementation | File Type | Strategy |
|---------------|-----------|----------|
| `HooksMerger` | `hooks.json` | Array concatenation per event key + deduplication by `(event, matcher, command)` tuple. Personal hooks always last. |
| `SettingsMerger` | `settings.json` | Recursive deep merge. Array values (e.g., `allowedTools`) merged by union. Personal keys win on collision. |
| `SectionMerger` | `CLAUDE.md`, `AGENTS.md` | Shared content first, then `<!-- aisync: personal content below -->` separator, then personal content. |

`HooksMerger` also implements `ExcludeAware` — an interface that allows the `PullCommand` to inject `hooks_exclude` entries from `config.yaml` after the merger is constructed (since config is loaded at pull time, not at startup).

### Atomic Apply (Two-Phase Commit)

The `AtomicApplyService` prevents partial updates to AI tool directories:

1. **Stage** — Write all incoming files to `~/.config/aisync/staging/<timestamp>/`.
2. **Journal** — Record pending operations (source paths, target paths, old/new SHA-256 checksums) in `journal.json`.
3. **Apply** — Move files from staging to final destinations via `os.Rename()`.
4. **Clear** — Delete journal after success.

On interruption, the next invocation detects the incomplete journal and resumes from where it left off. The `PullCommand` checks for pending journal recovery before starting a new pull.

### Bundle Sync

Per-tool project bundles allow syncing opaque directory trees (e.g. `~/.claude/projects/`, `~/.claude/plans/`) as age-encrypted tarballs so directory names that may contain company paths never appear in the git tree. Each `tools.<name>.bundles[]` entry in `config.yaml` declares a source directory and a target namespace.

Two bundle modes exist:

| Mode | Config value | Behavior |
|------|-------------|----------|
| `subdirs` (default) | `mode: subdirs` | Each immediate subdirectory is packaged as one `.age` tarball. Filename is `hmac_sha256(per_repo_key, name)[:16].age` where `per_repo_key` is HKDF-derived from the device's age identity (info string `aisync-bundle-name-v1`). The original name lives only inside the encrypted `_aisync-manifest.json`. Without the age identity, an attacker cannot compute or verify a filename for a guessed project name — closing the SHA-256 confirmation oracle that existed before. |
| `whole` | `mode: whole` | The entire source directory is one tarball — for loose-file directories like `plans/` or `todos/`. |

Two merge strategies control how pull reconciles bundles with local state:

| Strategy | Config value | Behavior |
|----------|-------------|----------|
| `mtime` (default) | `merge_strategy: mtime` | Keep the newer copy of each file by mtime; preserve local-only files; add bundle-only files. |
| `replace` | `merge_strategy: replace` | Overwrite local content unconditionally from the bundle. |

Cross-device deletion detection uses a per-device cache at `~/.cache/aisync/bundle-state.json` (`0600`, never committed). Pulls compute `removed = (cached) − (remote)` and prompt the user before deleting. Auto-removal is intentionally avoided.

Key files:

- `entities/bundle_manifest.go`, `entities/bundle_state.go` — value objects for bundle metadata and cross-device state.
- `entities/tool.go` — `BundleSpec`, `BundleMode`, `BundleMergeStrategy` definitions.
- `repositories/bundle_service.go` — `BundleService` interface (HashName, Bundle, Extract, MergeIntoLocal).
- `repositories/bundle_state_repository.go` — `BundleStateRepository` interface.
- `services/tar_age_bundle_service.go` — production implementation (tar+gzip+age).
- `repositories/json_bundle_state_repository.go` — persistent state store.
- `commands/push_bundles.go`, `commands/pull_bundles.go` — bundle pipeline integration.
- `commands/bundles_prune.go` — `PruneBundlesCommand` (`aisync bundles prune`).

### Platform-Specific Watch Service

`controllers/root.go` selects the watch implementation at startup:

- **Desktop (Linux/macOS/Windows)** — `FSNotifyWatchService` using OS-native events (`inotify`, `FSEvents`, `ReadDirectoryChangesW`).
- **Termux (Android)** — `PollingWatchService` with 30-second interval. Detected via `runtime.GOOS == "android"` or `ANDROID_ROOT` env var.

### Four-Layer Push Protection Stack

`aisync push` runs four orthogonal protections on every file. Any layer firing blocks the push. Each layer catches a different leak class — they compose, they don't overlap.

1. **Per-tool allowlist** (`entities/allowlist.go`) — A file is only syncable if its tool-relative path matches an entry in the compiled-in allowlist for that tool (e.g. `rules/**`, `agents/**`, `commands/**`, `hooks.json`, `settings.json`, `CLAUDE.md`). Tools without a compiled-in entry fall back to a conservative default. Users opt in to additional paths via `tools.<name>.extra_allowlist` in `config.yaml`. Replaces the old compiled deny-list — unknown content is now never synced rather than silently leaking when a vendor ships a new runtime directory.
2. **`.aisyncignore`** — Gitignore-syntax file for additional path exclusions on top of the allowlist. Additive, user-configurable.
3. **`.aisyncencrypt` + age** — Paths matching the patterns are written as ciphertext using `config.Encryption.Recipients`. Has two independent gates: pattern match AND non-empty recipients. Mismatched gates have previously caused silent plaintext commits.
4. **Content scanners** — Two scanners run in sequence on the unencrypted file map (the same map the encrypt path produced):
   - **`RegexSecretScanner`** (`services/secret_scanner.go`) — 15 compiled regexes for credential FORMATS (AWS keys, GitHub tokens, JWTs, private key blocks). Bypassable with `--skip-secret-scan`.
   - **`CompositeNDAChecker`** (`services/nda_content_checker.go`) — Composes three sources of forbidden terms: (a) explicit user list, age-encrypted at `<repo>/.aisync-forbidden.age`, default canonical-form substring matching (lowercase + NFKD-stripped + alphanumeric-only) so `ZestSecurity` catches every spacing/casing/separator/accent variant from one entry; (b) auto-derived from machine state via `services/auto_deriver.go` + `repositories/exec_git_inspector.go` — extracts terms from `git config --global user.email`, `git remote get-url origin` for repos under `~/Development`/`~/workspace`/etc. to depth 4, the `~/Development/dev.azure.com/<org>/<project>/` directory layout, and `~/.ssh/config` `Host <forge>-<alias>` entries; (c) compile-time heuristic shape checks (`services/nda_scanner.go:heuristicChecks`) — hardcoded home paths, WSL OneDrive paths, ADO org URLs, ssh-host alias patterns. Findings are tagged with the source (`user`, `auto-derived:<origin>`, `heuristic:<name>`) so the user can see exactly which knob fixes each hit. Bypassable with `--skip-nda-scan`. Per-device cache at `~/.cache/aisync/derived-terms.txt` (1h TTL, `0600`, never committed).

`PushCommand.Execute` runs the scanners both in the real-push path (`commitAndPush`) AND the dry-run path (`executeDryRun`) so `--dry-run` previews the actual blocks a real push would trigger.

The `aisync nda` command group (`add`, `remove`, `list`, `ignore`) manages the explicit forbidden list and the `nda.auto_derive_exclude` config entry. The `NDACommand` is in the domain layer and depends only on `ConfigRepository` + `ForbiddenTermsRepository`; the heuristic count is injected via constructor (`services.HeuristicCount()`) so the domain layer never imports infrastructure.

### Precedence Order

When applying files to local AI tool directories: personal files > last-listed source in `config.yaml` > earlier sources. A personal file with the same name as a shared file always wins.

## Testing Conventions

- `//go:build unit` tag on all unit test files — run with `go test -tags unit`
- BDD structure: `// given`, `// when`, `// then` comment blocks in every test
- Test names: `TestTypeName_MethodName_DescriptiveBehavior` (e.g., `TestHooksMerger_Merge_ConcatenatesArrays`)
- External test packages (e.g., `package commands_test`) — tests only access exported API
- `testify/assert` for assertions, `testify/require` for fatal preconditions
- Manual stubs in `test/doubles/mocks.go` — no mocking framework. Each stub captures call counts and arguments
- Unit tests use `t.Parallel()` with `t.Run()` subtests

## Configuration

Users name their sync repo `aifiles` (like chezmoi's `dotfiles`). Config lives at `<repo>/config.yaml` with sections for sync, encryption, tools (30+ AI tools, `aisync init` only enables ones it detects on the device), sources, watch, `hooks_exclude`, `nda` (auto_derive on/off, heuristics on/off, auto_derive_exclude per-device false positives, dev_roots override), and per-tool `bundles[]` specs (source dir, target namespace, mode, merge strategy). The `.aisyncencrypt` file declares which paths get age-encrypted. The `.aisyncignore` file uses gitignore syntax for user-configurable exclusions. The encrypted `.aisync-forbidden.age` (created on first `aisync nda add`) carries the explicit NDA forbidden-terms list — it lives at the repo root and travels between devices via the normal git flow, encrypted to the same age recipients as everything else. External sources are fetched as GitHub tarballs (zero API calls, no rate limiting) with ETag caching in `.aisync/state.json`.
