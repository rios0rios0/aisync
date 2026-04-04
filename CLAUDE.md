# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Project Does

aisync is a Go CLI tool that synchronizes AI coding assistant configurations (rules, agents, commands, hooks, skills, memories, settings) across multiple devices and multiple AI tools. It pulls shared rules from public external sources via Git tarball downloads, syncs personal configurations via a private Git repository, and encrypts sensitive data with age. It supports 30+ AI tools including Claude Code, Cursor, GitHub Copilot, Codex, Gemini CLI, Windsurf, and more.

## Build & Development Commands

```bash
make build          # Compile to bin/aisync (stripped)
make debug          # Compile with debug symbols
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

All unit test files use the `//go:build unit` build tag. Build times are ~2 seconds. Test suite runs in ~2 seconds.

## Architecture

Clean Architecture with Hexagonal (Ports & Adapters) design. No dependency injection framework — manual constructor wiring in the controller layer.

### Layer Structure

- **`cmd/aisync/`** — Entry point (`main.go`), sets up logrus and cobra root command
- **`internal/domain/`** — Business logic and contracts (no framework dependencies)
  - `commands/` — Use cases: `InitCommand`, `PullCommand`, `PushCommand`, `SyncCommand`, `DiffCommand`, `WatchCommand`, `StatusCommand`, `SourceCommand`, `KeyCommand`, `DeviceCommand`, `DoctorCommand`, `MigrateCommand`
  - `entities/` — Domain types: `Config`, `Source`, `Tool`, `Manifest`, `State`, `Journal`, `FileChange`, `Conflict`, `EncryptPatterns`, `IgnorePatterns`, `Formatter`
  - `repositories/` — Interfaces: `ConfigRepository`, `SourceRepository`, `ManifestRepository`, `StateRepository`, `GitRepository`, `JournalRepository`, `EncryptionService`, `ToolDetector`, `SecretScanner`, `DiffService`, `WatchService`, `Merger`, `ApplyService`, `ConflictDetector`
- **`internal/infrastructure/`** — Implementations
  - `controllers/root.go` — Cobra CLI wiring: creates all infrastructure instances, injects into domain commands, registers subcommands
  - `repositories/` — `YAMLConfigRepository`, `HTTPSourceRepository` (tarball fetch), `JSONManifestRepository`, `JSONStateRepository`, `JSONJournalRepository`, `GoGitRepository` (go-git wrapper)
  - `services/` — `AgeEncryptionService`, `HooksMerger`, `SettingsMerger`, `SectionMerger`, `AtomicApplyService`, `FSNotifyWatchService`, `PollingWatchService`, `FSDiffService`, `RegexSecretScanner`, `ConflictDetector`, `FSToolDetector`
  - `ui/` — `LipglossFormatter` for colored terminal output
- **`test/doubles/`** — Manual mock implementations of all domain interfaces

### Dependency Flow

```
main.go → controllers.NewRootCommand(version)
             ├── creates all infrastructure repos/services
             ├── creates all domain commands (injecting deps)
             ├── wires cobra subcommands
             └── returns root *cobra.Command
```

Dependencies always point inward: infrastructure → domain, never the reverse.

### Key External Libraries

| Library | Purpose |
|---------|---------|
| `filippo.io/age` | age encryption/decryption for personal files |
| `github.com/go-git/go-git/v5` | Pure Go git operations (clone, commit, push, pull) |
| `github.com/fsnotify/fsnotify` | OS-native filesystem event watching |
| `github.com/charmbracelet/lipgloss` | Colored terminal output |
| `github.com/spf13/cobra` | CLI framework |
| `github.com/sirupsen/logrus` | Structured logging |
| `github.com/rios0rios0/cliforge` | Self-update from GitHub Releases |
| `gopkg.in/yaml.v3` | YAML configuration parsing |

### CLI Commands

| Command | Domain Command | Description |
|---------|---------------|-------------|
| `aisync init [user]` | `InitCommand` | Create or clone aifiles repo |
| `aisync source add/remove/list/update/pin` | `SourceCommand` | Manage external sources |
| `aisync pull` | `PullCommand` | Fetch sources, merge, apply atomically |
| `aisync push` | `PushCommand` | Collect personal files, encrypt, commit, push |
| `aisync sync` | `SyncCommand` | Pull then push |
| `aisync diff` | `DiffCommand` | Preview changes with recency detection |
| `aisync watch` | `WatchCommand` | Real-time file monitoring with auto-push |
| `aisync status` | `StatusCommand` | Show sync state and tool status |
| `aisync key generate/import/export/add-recipient` | `KeyCommand` | Age encryption key management |
| `aisync device list/rename/remove` | `DeviceCommand` | Device registry management |
| `aisync doctor` | `DoctorCommand` | Diagnose configuration issues |
| `aisync migrate` | `MigrateCommand` | Import existing files into aifiles structure |
| `aisync self-update` | cliforge | Update binary from GitHub Releases |

### Key Design Patterns

- **Merge strategies**: `hooks.json` uses array concatenation with deduplication; `settings.json` uses deep merge; `CLAUDE.md`/`AGENTS.md` use section concatenation with `<!-- aisync: personal content below -->` separator
- **Atomic apply**: Two-phase commit with journal — stage files to temp dir, write journal, then move to final destinations. Recovery on next invocation if interrupted.
- **Deny-list**: Compiled-in patterns for credentials/sessions/plugins that cannot be overridden by the user
- **Precedence**: Personal files override shared files with the same name. Last source in config wins on collision.
- **Encryption**: Files matching `.aisyncencrypt` patterns are encrypted with age before git commit and decrypted after git pull

## Testing Conventions

- BDD structure with `// given`, `// when`, `// then` comment blocks
- Test names: descriptive function names like `TestHooksMerger_Merge_ConcatenatesArrays`
- Unit tests use `//go:build unit` build tag
- Assertions via `testify/assert`
- Mock doubles in `test/doubles/mocks.go` — manual struct-based stubs storing call counts and captured args
- 377 tests, 80%+ coverage on all business logic packages

## Configuration

The `aifiles` repo convention: users name their sync repo `aifiles` (like chezmoi's `dotfiles`). Config is at `<repo>/config.yaml` with sections for sync, encryption, tools (30+), sources, watch, and hooks_exclude. The `.aisyncencrypt` file at repo root declares which paths get age-encrypted. The `.aisyncignore` file uses gitignore syntax for user-configurable exclusions.
