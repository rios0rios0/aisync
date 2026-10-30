# AGENTS.md

This file provides guidance to AI coding agents (Claude Code, Codex, Cursor, GitHub Copilot) when working with code in this repository.

## What This Project Does

aisync is a Go CLI tool that synchronizes AI coding assistant configurations (rules, agents, commands, hooks, skills, memories, settings) across multiple devices and 30+ AI tools. It pulls shared rules from public external sources via Git tarball downloads, syncs personal configurations via a private Git repository, and encrypts sensitive data with age.

## Build & Development Commands

```bash
make build          # Compile to bin/aisync (stripped with -s -w)
make debug          # Compile with debug symbols (-gcflags "-N -l")
make run            # go run ./cmd/aisync
make install        # Build and copy to ~/.local/bin/aisync
make lint           # golangci-lint with the pipelines config; calling golangci-lint directly skips that config
make test           # Full test suite via pipeline scripts
make sast           # Security analysis (CodeQL, Semgrep, Trivy, Hadolint, Gitleaks)
```

`lint`, `test`, and `sast` come from the `rios0rios0/pipelines` checkout at `SCRIPTS_DIR` (default `~/Development/github.com/rios0rios0/pipelines`). The Makefile `-include`s it, so those targets do not exist without that checkout.

Run a single test during development:
```bash
go test -run "TestHooksMerger_Merge" ./internal/infrastructure/services/
```

Unit test files carry no build tag, so plain `go test ./...` runs them. Build and test suite each run in ~2 seconds.

## Architecture

Clean Architecture with Hexagonal (Ports & Adapters) design. Dependencies always point inward: infrastructure depends on domain interfaces, never the reverse. No DI framework — manual constructor wiring in `controllers/root.go`.

### Layer Boundaries

- **`cmd/aisync/`** — Entry point. Sets up logrus, creates the cobra root command via `controllers.NewRootCommand(version)`, and calls `Execute()`.
- **`internal/domain/`** — Zero framework dependencies. Contains commands (use cases), entities (value objects/aggregates), and repository interfaces (ports).
- **`internal/infrastructure/`** — Implements domain interfaces. Contains `controllers/` (cobra wiring + DI), `repositories/` (YAML/JSON/HTTP/Git persistence), `services/` (encryption, merging, watching, scanning), and `ui/` (lipgloss formatter).
- **`test/doubles/`** — Manual struct-based stubs for all domain interfaces. Each stub stores call counts and captured args for assertion — no mocking framework.

### How Dependency Injection Works

`controllers/root.go` is the composition root. `NewRootCommand(version)` creates every infrastructure instance, injects them into domain command constructors, and wires cobra subcommands. This is the only place where concrete types are referenced — domain commands accept only interfaces.

### Adding a New Command

Adding a command touches 4 locations:

1. **`internal/domain/commands/`** — Create `foo.go` with a `FooCommand` struct. Constructor accepts domain interfaces. Single `Execute()` method with business logic.
2. **`internal/infrastructure/controllers/root.go`** — Instantiate `FooCommand` with infrastructure deps, create a `newFooSubcmd()` helper that wraps it in a `cobra.Command`, add to `root.AddCommand(...)`.
3. **`test/doubles/mocks.go`** — Add stubs for any new domain interfaces (if introduced).
4. **`internal/domain/commands/foo_test.go`** — Unit tests using stubs from `test/doubles/`.

### Merger Polymorphism

The `Merger` interface has two implementations, each handling a different single-file merge strategy:

| Implementation | File Type | Strategy |
|---------------|-----------|----------|
| `HooksMerger` | `hooks.json` | Array concatenation per event key + deduplication by `(event, matcher, command)` tuple. Personal hooks always last. |
| `SettingsMerger` | `settings.json` | Recursive deep merge. Array values (e.g., `allowedTools`) merged by union. Personal keys win on collision. |

Any other file a source delivers is applied whole, so a `CLAUDE.md` opted in through `extra_allowlist` is overwritten, never merged.

`HooksMerger` also implements `ExcludeAware` — an interface that allows the `PullCommand` to inject `hooks_exclude` entries from `config.yaml` after the merger is constructed (since config is loaded at pull time, not at startup).

### Atomic Apply (Two-Phase Commit)

The `AtomicApplyService` prevents partial updates to AI tool directories:

1. **Stage** — Write all incoming files to `~/.config/aisync/staging/<timestamp>/`. `Stage` converts text files to LF line endings (shared files); `StageVerbatim` keeps every byte (personal files and conversations, which must read back exactly as their recorded base).
2. **Journal** — Record pending operations (source paths, target paths, old/new SHA-256 checksums) in `journal.json`.
3. **Apply** — Move files from staging to final destinations via `os.Rename()`.
4. **Clear** — Delete journal after success.

On interruption, the next invocation detects the incomplete journal and resumes from where it left off. The `PullCommand` checks for pending journal recovery before starting a new pull.

### Three-Way Personal Sync

`commands/personal_sync.go` (`PersonalSyncer`) reconciles personal files between each enabled tool directory and `personal/<tool>/` in the sync repo. Pull, push, sync, diff, and status all go through it; external-source (shared) files keep their own flow in `pull.go`. `status` counts the plan with `Pending`, so it reports exactly what a sync would send, apply, or ask about, never a second opinion.

- **Base**: the tool's `.aisync-manifest.json` entries with `source: personal` record the checksum each path had when this device last synced it. The manifest is device-local (never synced) and is only rewritten when a base entry changes.
- **Classification**: `entities.Reconcile(local, remote, base)` (`entities/sync_plan.go`) is a pure decision table returning a `SyncEntry` per path: `push`, `pull`, `delete-repo`, `delete-local`, `forget`, `none`, or `conflict` with a `ConflictReason`. Without a base (first sync), differing copies are `both-added` conflicts and nothing is inferred as deleted.
- **Phases**: plan every tool → resolve every conflict (a "for all" answer, `--prefer local|remote|newer`, or `PromptService.PromptSyncConflict`) → execute → save bases. Quitting at a prompt returns `ErrSyncAborted` before anything is written. Execution dispatches through the `actionHandlers` map; incoming files go through the atomic apply journal in one batch.
- **Direction**: `push` runs outgoing actions, `pull` incoming ones, and `sync` both in its push phase (its pull phase passes `SkipPersonal`, so each conflict is asked once). A conflict the user resolves runs in full whatever the direction.
- **Conflict detail**: local mtime, the last repo commit touching the path under either stored name, skipping commits with the `Aisync-Change: encryption-only` trailer (`RepoHistory`, `git log`), and a unified diff (`TextDiffer`, `services/unified_differ.go` on top of go-diff). The huh prompt pages long diffs; without a terminal it reads answers from stdin, one line per prompt through a single shared reader, and skips at end of input.
- **Scope**: the same filter applies to all three sides: the tool allowlist, `.aisyncignore`, paths owned by external sources, and the device's exclusions (see "Device Exclusions"). Out-of-scope paths are never touched. Local files are read through an `os.Root`, so a symlink is followed only when it is relative and stays inside the tool directory.
- **Encryption**: `.age` copies are decrypted for comparison, so an unchanged encrypted file is never rewritten; a copy this device cannot decrypt is left alone on both sides. The owner-execute bit is carried to the repo and restored on pull. Before reconciling, a push or sync also encrypts in place every plaintext file under `personal/` that matches `.aisyncencrypt`, including those of tools not installed on the device, so a pattern added later protects the files already committed (`commands/personal_encryption.go`). When the repo was clean, that change is its own commit with the `Aisync-Change: encryption-only` trailer, which history lookups skip so each file keeps the date of its last real change; otherwise it rides in the scanned main commit.

### Transcript Sync

`commands/transcript_sync.go` (`TranscriptSyncer`) syncs the files selected by a tool's `transcripts` spec (`entities/transcripts.go`: `dir`, `logs`, `files`, `max_age`) through a snapshot branch instead of `personal/<tool>/`. It runs after the personal sync in `pull` (incoming), `push` (outgoing, after the main commit is pushed), and `sync` (both).

- **Storage** (`services/git_transcript_service.go`, `GitTranscriptService`): a bare repo at `<repo>/.aisync/transcripts.git` (device-local). Fetch is `--depth=1`; publish writes gzip+age blobs named `<tool>/<hmac>.age` (HKDF info `aisync-transcript-name-v1`) plus an encrypted `index.age`, builds the tree with `git mktree`, commits with no parent, and pushes `--force-with-lease`. Unchanged files reuse their blob, so only changed conversations upload. Objects of replaced snapshots are pruned.
- **Concurrency**: a stale lease (local tracking ref moved, or `stale info` from the push) is `repositories.ErrSnapshotMoved`; the syncer fetches and merges again, up to three attempts.
- **Merge**: `entities.Reconcile` over local files, the snapshot index, and a per-device base (`.aisync/transcripts-base.json`). Logs expire when their newest copy on either side is older than `max_age` (judged on both sides, so expiry is never read as a deletion). Log conflicts resolve without prompting (change beats deletion; a copy that extends the other wins; otherwise the newer). Other files share the conflict review through `decideConflict`. Pulled files keep the publisher's mtime. A pull never publishes.

### Device Exclusions

`exclude` in aisync's config directory (`~/.config/aisync/exclude`, `%APPDATA%\aisync\exclude` on Windows, or the file `AISYNC_EXCLUDE_FILE` names; `controllers.DefaultExcludePath`) lists the paths this device opts out of, one `<tool>/<pattern>` per line, `*` for every tool, matched against the tool-relative path with the allowlist's anchored matcher (`entities/exclude_patterns.go`). It lives outside the sync repo because `config.yaml` and `.aisyncignore` are committed: the point is one device keeping its own copy of a file every other device shares.

- **Loading**: `repositories.FileExcludeRepository` reads the file on every `Load`, so `watch` picks up an edit. A missing file excludes nothing; one that exists but cannot be read is an error, which stops pull, push and sync before they write to a tool directory or commit anything; a line without a tool is ignored with a warning, once per process.
- **Wiring**: `root.go` hands the repository to the `PersonalSyncer`, the `TranscriptSyncer`, `PullCommand`, `DiffCommand` and `StatusCommand` through `WithExcludes`; a command built without it excludes nothing, which is what the tests that do not exercise exclusions get.
- **Personal files**: the exclusion joins `inScope`, so the path drops out of the local scan, the repo scan and the base alike, and is never pulled, pushed, deleted or conflicted. Its manifest base is left as it was, so removing the line resumes the three-way merge.
- **Transcripts**: a file matches as `<tool>/<transcripts.dir>/<path>`. It is neither scanned nor merged, stays in the published snapshot for every other device, and keeps its base entry.
- **External sources**: pull drops excluded `shared/<tool>/<path>` files before applying them, and `detectDeletions` skips them, so a file applied before it was excluded is left in place rather than deleted; it simply stops being tracked in the manifest. `diff` drops them before computing the shared diff.
- **Not covered**: bundles, and the `watch` event filter (an event on an excluded file starts a push that changes nothing). `EncryptStoredPlaintext` still encrypts a plaintext repo copy of an excluded path in place, as it does for tools not installed on the device: the content is unchanged.

### Bundle Sync

Per-tool project bundles allow syncing opaque directory trees (e.g. `~/.claude/projects/`, `~/.claude/plans/`) as age-encrypted tarballs so directory names that may contain company paths never appear in the git tree. Each `tools.<name>.bundles[]` entry in `config.yaml` declares a source directory and a target namespace.

Two bundle modes exist:

| Mode | Config value | Behavior |
|------|-------------|----------|
| `subdirs` (default) | `mode: subdirs` | Each immediate subdirectory is packaged as one `.age` tarball. Filename is `hmac_sha256(per_repo_key, name)[:16].age` where `per_repo_key` is HKDF-derived from the device's age identity (info string `aisync-bundle-name-v1`). The original name lives only inside the encrypted `_aisync-manifest.json`. Without the age identity, an attacker cannot compute or verify a filename for a guessed project name. Bundle ciphertext is also padded with cryptographic-random bytes (appended after the gzip end marker; `Multistream(false)` ignores them on extract) up to the smallest power-of-2 bucket from 16 KiB to 128 MiB, so two bundles in the same bucket are byte-length-indistinguishable in the public-clone view. |
| `whole` | `mode: whole` | The entire source directory is one tarball — for loose-file directories like `plans/` or `todos/`. |

Two merge strategies control how pull reconciles bundles with local state:

| Strategy | Config value | Behavior |
|----------|-------------|----------|
| `mtime` (default) | `merge_strategy: mtime` | Keep the newer copy of each file by mtime; preserve local-only files; add bundle-only files. |
| `replace` | `merge_strategy: replace` | Overwrite local content unconditionally from the bundle. |

Cross-device deletion detection uses a per-device cache at `~/.cache/aisync/bundle-state.json` (`0600`, never committed). Pulls compute `removed = (cached) − (remote)` and prompt the user before deleting. Auto-removal is intentionally avoided.

### Four-Layer Push Protection Stack

`aisync push` runs four orthogonal protections on every file. Any layer firing blocks the push. Each layer catches a different leak class — they compose, they don't overlap.

1. **Per-tool allowlist** (`entities/allowlist.go`) — A file is only syncable if its tool-relative path matches an entry in the compiled-in allowlist for that tool (e.g. `rules/**`, `agents/**`, `commands/**`, `hooks.json`, `settings.json`). The home-level instruction files (`CLAUDE.md` and `AGENTS.md` at a tool's root) are deliberately left out: a dotfiles manager renders them per machine, and a synced copy would overwrite, resurrect or duplicate it. Tools without a compiled-in entry fall back to a conservative default. `ToolDenylists` and `CommonDenylist` exclude tool-managed content inside allowlisted directories (Claude's account-synced `skills/synced/` and `skills/.trash/`, Codex's bundled `skills/.system/`, `.gitkeep` placeholders). Users opt in to additional paths via `tools.<name>.extra_allowlist` in `config.yaml`, which also overrides the denylists. Unknown content is never synced, so a new runtime directory a vendor ships cannot leak.
2. **`.aisyncignore`** — Gitignore-syntax file for additional path exclusions on top of the allowlist. Additive, user-configurable.
3. **`.aisyncencrypt` + age** — Paths matching the patterns are written as ciphertext using `config.Encryption.Recipients`. Has two independent gates: pattern match AND non-empty recipients. Mismatched gates have previously caused silent plaintext commits.
4. **Content scanners** — Two scanners run in sequence on the plaintext the commit is about to publish: every uncommitted, non-`.age` file under `personal/` (`GitRepository.ChangedFiles`), whichever command wrote it. Files already committed are not rescanned, so an old false positive does not block unrelated pushes:
   - **`RegexSecretScanner`** (`services/secret_scanner.go`) — 15 compiled regexes for credential FORMATS (AWS keys, GitHub tokens, JWTs, private key blocks). Bypassable with `--skip-secret-scan`.
   - **`CompositeNDAChecker`** (`services/nda_content_checker.go`) — Composes three sources of forbidden terms: (a) explicit user list, age-encrypted at `<repo>/.aisync-forbidden.age`, default canonical-form substring matching (lowercase + NFKD-stripped + alphanumeric-only) so `ContosoSecurity` catches every spacing/casing/separator/accent variant from one entry; (b) auto-derived from machine state via `services/auto_deriver.go` + `repositories/exec_git_inspector.go` — extracts terms from `git config --global user.email`, `git remote get-url origin` for repos under `~/Development`/`~/workspace`/etc. to depth 4, the `~/Development/dev.azure.com/<org>/<project>/` directory layout, and `~/.ssh/config` `Host <forge>-<alias>` entries; (c) compile-time heuristic shape checks (`services/nda_scanner.go:heuristicChecks`) — hardcoded home paths, WSL OneDrive paths, ADO org URLs, ssh-host alias patterns. Findings are tagged with the source (`user`, `auto-derived:<origin>`, `heuristic:<name>`) so the user can see exactly which knob fixes each hit. Bypassable with `--skip-nda-scan`. Per-device cache at `~/.cache/aisync/derived-terms.txt` (1h TTL, `0600`, never committed).

`PushCommand.Execute` runs the scanners both in the real-push path (`commitAndPush`) AND the dry-run path (on the plaintext the PersonalSyncer would write) so `--dry-run` previews the actual blocks a real push would trigger. A push first updates the sync repo from its remote so other devices' changes are seen as conflicts, and a failed `git push` is an error (the commit stays local and the next push retries it).

The `aisync nda` command group (`add`, `remove`, `list`, `ignore`) manages the explicit forbidden list and the `nda.auto_derive_exclude` config entry. The `NDACommand` is in the domain layer and depends only on `ConfigRepository` + `ForbiddenTermsRepository`; the heuristic count is injected via constructor (`services.HeuristicCount()`) so the domain layer never imports infrastructure.

### Precedence Order

When applying files to local AI tool directories: personal files > last-listed source in `config.yaml` > earlier sources. A personal file with the same name as a shared file always wins.

### Age Identity Provisioning

The `aisync key` command group (`commands/key.go`) manages the age identity: `generate`, `import <path>`, `export`, `add-recipient <public-key>`, and `import-from-op`. `import-from-op` fetches the `AGE-SECRET-KEY-1...` value from a 1Password item (field labelled `private key`) via the official `op` CLI, writes it to `encryption.identity`, and appends the derived public key to `encryption.recipients`. It is strictly opt-in — it errors unless `encryption.op.enabled: true`. Config lives under `encryption.op` (`enabled`, `vault`, `item`; `item` defaults to `aisync.age`). The `op` binary owns the entire auth/session model (biometric prompt, SSH-agent, or service-account token); aisync never touches the master password. Implementation: `repositories/op_cli_secret_repository.go` behind the domain `OpSecretRepository` port.

## Testing Conventions

- No build tag on unit test files, so `go test ./...`, the IDE and the linter all see them; reserve `//go:build integration` for a test that needs real infrastructure
- BDD structure: `// given`, `// when`, `// then` comment blocks in every test
- Test names: `TestTypeName_MethodName_DescriptiveBehavior` (e.g., `TestHooksMerger_Merge_ConcatenatesArrays`)
- External test packages (e.g., `package commands_test`) — tests only access exported API
- `testify/assert` for assertions, `testify/require` for fatal preconditions
- Manual stubs in `test/doubles/mocks.go` — no mocking framework. Each stub captures call counts and arguments
- Unit tests call `t.Parallel()` at the top of the test and in every `t.Run()` subtest; `paralleltest` and `tparallel` fail CI otherwise. A test that calls `t.Setenv` cannot be parallel, so such sub-tests live in their own sequential function (e.g. `TestInitCommand_Execute_WithHomeDirectory`)
- Git-backed tests create repositories with `initTempRepo`, which turns off commit signing and global hooks so the tests never depend on the developer's git setup

## Configuration

Users name their sync repo `aifiles` (like chezmoi's `dotfiles`). Config lives at `<repo>/config.yaml` with sections for sync, encryption (age `identity` path, `recipients`, and an optional `op` subsection), tools (30+ AI tools, `aisync init` only enables ones it detects on the device), sources, watch, `hooks_exclude`, `nda` (auto_derive on/off, heuristics on/off, auto_derive_exclude per-device false positives, dev_roots override), and per-tool `bundles[]` specs (source dir, target namespace, mode, merge strategy). The `.aisyncencrypt` file declares which paths get age-encrypted. The `.aisyncignore` file uses gitignore syntax for user-configurable exclusions. The encrypted `.aisync-forbidden.age` (created on first `aisync nda add`) carries the explicit NDA forbidden-terms list — it lives at the repo root and travels between devices via the normal git flow, encrypted to the same age recipients as everything else. External sources are fetched as GitHub tarballs (zero API calls, no rate limiting) with ETag caching in `.aisync/state.json`.

<!-- chlog:start -->
## Changelog (chlog) — MANDATORY

If the repository you are working in uses chlog (a `.chlog.yaml` or `.chlog.yml`
config file, or a `.changes/` directory, exists at the project root), the
following is binding and ALWAYS applies: whenever you make ANY change, you MUST
create a changelog fragment as part of the same change — automatically, without
being asked, before committing.

- Do NOT edit CHANGELOG.md directly; it is generated from fragments.
- Create the fragment with:
  `chlog new --kind <Kind> --body '<past-tense description>'`
- Write an apostrophe inside the single-quoted body as `'\''`.
- Valid kinds: Added, Changed, Deprecated, Removed, Fixed, Security
- Choose the kind that best matches the change (e.g., new feature → Added,
  bug fix → Fixed, behavior change → Changed, removal → Removed, security fix → Security).
- If the change is backward-INCOMPATIBLE with the public API (a breaking
  change), you MUST add the `--breaking` flag:
  `chlog new --kind <Kind> --breaking --body '<past-tense description>'`.
  This is the ONLY thing that triggers a major version bump — the kind alone
  never does (per SemVer, major = incompatible change). When unsure whether a
  change breaks compatibility, ask the user instead of guessing.
- Fragments are YAML files in `.changes/unreleased/`; stage them with your commit.
- `chlog check` fails the build when a fragment is missing — never skip it.
<!-- chlog:end -->
