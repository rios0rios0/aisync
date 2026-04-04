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
             ├── creates repos: YAMLConfigRepo, HTTPSourceRepo, JSONManifestRepo, JSONStateRepo, GoGitRepo, JSONJournalRepo
             ├── creates services: AgeEncryptionSvc, HooksMerger, SettingsMerger, SectionMerger, AtomicApplySvc, WatchSvc, ...
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

### Platform-Specific Watch Service

`controllers/root.go` selects the watch implementation at startup:

- **Desktop (Linux/macOS/Windows)** — `FSNotifyWatchService` using OS-native events (`inotify`, `FSEvents`, `ReadDirectoryChangesW`).
- **Termux (Android)** — `PollingWatchService` with 30-second interval. Detected via `runtime.GOOS == "android"` or `ANDROID_ROOT` env var.

### Two-Tier Ignore System

- **Tier 1: Compiled-in deny-list** (`entities/denylist.go`) — Hardcoded patterns for credentials, OAuth tokens, session transcripts, plugin caches. Cannot be overridden. Safety net against accidental secret leaks.
- **Tier 2: User-configurable `.aisyncignore`** — Gitignore-syntax file for additional exclusions. Additive with the deny-list.

The `RegexSecretScanner` (15 compiled regex patterns) runs before every push and blocks if secrets are found in non-encrypted files.

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

Users name their sync repo `aifiles` (like chezmoi's `dotfiles`). Config lives at `<repo>/config.yaml` with sections for sync, encryption, tools (30+ AI tools auto-detected on init), sources, watch, and `hooks_exclude`. The `.aisyncencrypt` file declares which paths get age-encrypted. The `.aisyncignore` file uses gitignore syntax for user-configurable exclusions. External sources are fetched as GitHub tarballs (zero API calls, no rate limiting) with ETag caching in `.aisync/state.json`.
