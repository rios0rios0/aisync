# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

When a new release is proposed:

1. Create a new branch `bump/x.x.x` (this isn't a long-lived branch!!!);
2. The Unreleased section on `CHANGELOG.md` gets a version number and date;
3. Open a Pull Request with the bump version changes targeting the `main` branch;
4. When the Pull Request is merged, a new Git tag must be created using <LINK TO THE PLATFORM TO OPEN THE PULL REQUEST>.

Releases to productive environments should run from a tagged version.
Exceptions are acceptable depending on the circumstances (critical bug fixes that can be cherry-picked, etc.).

## [Unreleased]

### Changed

- changed the Go module dependencies to their latest versions

## [1.3.0] - 2026-04-30

### Added

- added `aisync key import-from-op` to import an age identity from a 1Password item via the official `op` CLI. Activated by adding an `encryption.op` block to `config.yaml` (`enabled`, `vault`, optional `item` defaulting to `aisync.age`); the command reads the `private key` field of the named item and writes it to the same identity path every other key operation uses (`encryption.identity` → `AISYNC_KEY_FILE` → `~/.config/aisync/key.txt`), then appends the derived public key to `recipients`, which let new devices restore the existing aifiles age identity without copying the private key file between machines manually
- added `EncryptionService.ImportKeyContent` for write-from-memory imports so secret material fetched from `op` (and future non-file sources) never lands in a temp file on disk

### Changed

- changed the Go module dependencies to their latest versions

## [1.2.1] - 2026-04-29

### Changed

- changed the Go module dependencies to their latest versions

## [1.2.0] - 2026-04-28

### Added

- added `aisync init --refresh-scaffolding` for upgrading older aifiles repos whose scaffolding pre-dates recent template changes. The flag overwrites `.gitignore`, `.aisyncignore`, and `.aisyncencrypt` with the latest default templates while leaving `config.yaml`, repo content, and Git state untouched. Without this flag, users who initialised their repo before the comprehensive `.aisyncencrypt` pattern list landed had no in-tool way to pick up new defaults like `personal/**/mcp.json` or `personal/**/*.key`, leaving credentials in plaintext on every push

### Changed

- changed `YAMLConfigRepository.Save` to emit string values in single-quoted style (`branch: 'main'`) while keeping booleans, integers, floats, and map keys unquoted. Aligns the file `aisync` writes during `init`, `source add`, `nda add`, `key add-recipient`, and other config-mutating commands with the project's YAML convention. Round-trips with `Load` are preserved verbatim — no observed values change

### Fixed

- fixed `aisync init` shipping an `.aisyncignore` template that did not exclude tool-managed `.gitignore` files. When `personal/cursor/.gitignore` (synced verbatim from `~/.cursor/.gitignore`, which begins with `*` to ignore everything in the tool dir) landed in a freshly-initialised aifiles repo, it silently caused git to ignore freshly-encrypted `personal/cursor/mcp.json.age` and any sibling `.age` file inside the same directory — `aisync push` would report "1 file collected" while the encrypted ciphertext never made it into the commit. The default `.aisyncignore` now contains `**/.gitignore`, forcing the repo root `.gitignore` to be the single source of truth for git-ignore semantics inside aifiles

### Security

- changed bundle ciphertext to be padded up to a fixed size bucket before age encryption. Each gzipped tarball is padded with cryptographic-random bytes (appended after the gzip end marker; gzip reader uses `Multistream(false)` on extract so the trailing bytes are ignored) up to the smallest entry in `bundleSizeBuckets` that fits — powers of 2 from 16 KiB to 128 MiB. Without this, an attacker reading bundle file sizes in the public-clone view could profile activity per project (a 9 MiB bundle is more interesting than an 88 KiB one) without decrypting anything; with this, two bundles in the same bucket are byte-length-indistinguishable. Smallest realistic project (88 KiB before padding) now lands at 128 KiB on disk; the largest tested case (9.3 MiB) lands at 16 MiB. Storage overhead per bundle averages ~25% (between an actual size and the next power of 2). Padding bytes are random rather than deterministic so two pushes of identical content still produce distinct ciphertext bytes — closing an equality-comparison oracle that would otherwise let an attacker correlate pushes across devices
- changed the bundle filename derivation from `sha256(directory_name)[:16]` to `HMAC-SHA256(per-repo-key, directory_name)[:16]`, where the per-repo key is HKDF-derived from the device's age identity at `config.Encryption.Identity` (info string `aisync-bundle-name-v1`). The old scheme was a confirmation oracle: an attacker who guessed a project directory name (e.g. `~/Development/dev.azure.com/<company>/<project>`) could compute the hash and verify against `personal/<tool>/<bundle>/*.age` filenames in the public-clone view, even though they could not decrypt the bundle contents. With HMAC, the attacker needs the age private key to compute or verify any bundle name — the same key required to decrypt the bundle, so the oracle is closed. **BREAKING CHANGE:** the `BundleService.HashName` interface signature changed from `HashName(name string) string` to `HashName(name, identityPath string) (string, error)`. Existing aifiles repos initialised under the SHA-256 scheme will produce orphaned `.age` files on the next push (their filenames won't match the HMAC names the new push generates). Either run `aisync bundles prune` after the next push to remove orphans, or wipe-and-recreate the repo to start clean. A future migration command will rename old bundles in place; until then, manual cleanup is required

## [1.1.1] - 2026-04-28

### Changed

- changed the Go module dependencies to their latest versions
- refreshed `CLAUDE.md` and `.github/copilot-instructions.md` to document the bundle sync architecture added in 1.1.0 and fix the `RegexSecretScanner` source filename reference (`secret_scanner.go`, not `regex_secret_scanner.go`)

### Fixed

- fixed `aisync init` not creating a `.gitignore` in the aifiles repo, which left `.aisync/state.json` (per-device ETag cache) and `.aisync/journal.json` (per-pull atomic-apply state) showing as untracked on every device. The default `.gitignore` excludes `.aisync/*` while preserving `.aisync/.gitkeep` so the directory itself stays tracked. Both the `executeCreate` and `executeClone` paths now backfill the file when missing

## [1.1.0] - 2026-04-25

### Added

- added `~/.claude/scripts/**` to the compiled-in claude allowlist so personal helper scripts referenced from `hooks.json` can sync alongside their callers without per-user `extra_allowlist` configuration.
- added `~/.cursor/mcp.json` to the compiled-in cursor allowlist, paired with a default `personal/**/mcp.json` entry in the `aisync init` `.aisyncencrypt` template so MCP server configs (which often carry API tokens or workspace-leaking paths) sync as age ciphertext on every fresh device.
- added bundle-aware push and pull pipelines. `aisync push` walks every configured `BundleSpec`, produces one bundle per source subdirectory, and writes the ciphertext under `personal/<tool>/<target>/`. The dry-run summary counts bundles toward both the file total and the encrypted total. `aisync pull` decrypts every bundle that arrived from the remote and merges its files into the matching local source directory using a configurable merge strategy.
- added cross-device deletion detection backed by a per-device cache at `~/.cache/aisync/bundle-state.json` (mode `0600`, never committed). Pulls compute `removed = (cached hashes) - (remote hashes)` and prompt the user before removing each local source directory whose bundle disappeared upstream. Auto-removal is intentionally avoided so a transient `rm -rf` on one machine cannot turn into a remote nuke.
- added per-tool project-bundle sync. Each `tools.<name>.bundles[]` entry in `config.yaml` declares a tool-relative source directory whose immediate subdirectories are each packaged into one age-encrypted gzip-compressed tarball under `personal/<tool>/<target>/<sha256(name)[:16]>.age`. The bundle filename is intentionally a hash so directory names that may contain project paths or company codenames never appear in the git tree. The internal `_aisync-manifest.json` carries the original directory name so pull-side code can restore it locally.
- added the `aisync bundles prune` subcommand: walks every configured bundle target, asks the user about each `.age` file whose source directory no longer exists locally, and deletes the confirmed orphans from the sync repo (the deletion is committed by the next push).
- added the `BundleModeWhole` bundle mode (`tools.<name>.bundles[].mode: whole`) for source directories that contain loose files instead of subdirectories (e.g. `~/.claude/plans/`, `~/.claude/todos/`). In whole mode the entire source is packaged as one age-encrypted tarball, complementing the existing default `subdirs` mode that produces one bundle per immediate subdirectory.
- added the `mtime` and `replace` bundle merge strategies. `mtime` (default) keeps whichever copy of a file has the newer modification time, preserves files that exist only locally, and adds files that exist only in the bundle — the right semantics for memory-style append-mostly content where two devices may both append independently. `replace` overwrites local content unconditionally for users who want bundle-first semantics.

### Fixed

- fixed `ParseEncryptPatterns` to extract the first whitespace-separated token from each line, so `.gitattributes`-style rows like `personal/*/settings.local.json    encrypt` resolve to the path glob instead of being stored as a literal that never matches. Before this fix, a `.aisyncencrypt` shipped with the action keyword silently produced zero encrypted files on push.

## [1.0.3] - 2026-04-24

### Changed

- changed the Go module dependencies to their latest versions

## [1.0.2] - 2026-04-17

### Changed

- changed the Go module dependencies to their latest versions

## [1.0.1] - 2026-04-16

### Changed

- changed the Go module dependencies to their latest versions

## [1.0.0] - 2026-04-15

### Added

- added `.github/copilot-instructions.md` with architecture, testing, security, and anti-pattern guidance so GitHub Copilot and other AI assistants produce code matching the project's Clean Architecture layout, BDD test discipline, and encrypt/scan gates
- added `aisync push --skip-nda-scan` and `--dry-run` flags (the existing `--skip-secret-scan` is unchanged) so emergency bypasses and preview runs are explicit and visible
- added `nda.auto_derive`, `nda.heuristics`, `nda.auto_derive_exclude`, and `nda.dev_roots` keys to `config.yaml` for users who need to tune the auto-derivation pipeline (defaults: auto_derive on, heuristics on, exclude empty, dev_roots empty so the built-in defaults apply)
- added `Tool.ExtraAllowlist` (`tools.<name>.extra_allowlist` in `config.yaml`) — a per-tool user-extensible list of gitwildmatch patterns that complements the compiled-in allowlist. Lets users opt in to syncing additional paths for a given tool (e.g. `my-research/**`) without patching aisync source code
- added a fourth content-aware protection layer to `aisync push`: the **NDA scanner** runs on every unencrypted file alongside the credential regex scanner and blocks the push if any file contains a forbidden term. Three sources feed the scanner simultaneously: an explicit user-managed list stored encrypted in the sync repo, terms auto-derived from machine state (git remotes, gitconfig user.email, ssh-config host aliases, dev-directory layouts), and a small set of compile-time heuristic shape checks (hardcoded home paths, WSL OneDrive paths, ADO/GitHub org URLs, ssh-host alias patterns). The whole stack exists so the user never has to manually grep `~/.claude/rules/**` for company names, project codenames, or customer-environment paths before pushing
- added auto-derivation from machine state: every push (unless `nda.auto_derive: false` is set in `config.yaml`) extracts forbidden-term candidates from `git config --global user.email` (skipping a public-free-mail allowlist of gmail/outlook/icloud/etc.), `git remote get-url origin` for every repo found under `~/Development`, `~/workspace`, `~/code`, `~/src`, `~/projects`, and `~/go/src` to depth 4, the `~/Development/dev.azure.com/<org>/<project>/` directory layout (and equivalent `github.com`/`gitlab.com`/`bitbucket.org` two-level walks), and `~/.ssh/config` `Host <forge>-<alias>` entries. The user's own GitHub login (from `gh api user`) is filtered out so personal repos are not flagged. Results are cached at `~/.cache/aisync/derived-terms.txt` (`0600`, never committed) for one hour to keep push latency negligible
- added automatic recipient registration in `aisync init` create mode when an age identity already exists at the configured path: the public key is derived via `ExportPublicKey` and added to `config.Encryption.Recipients` so fresh configs on machines with a pre-existing key immediately encrypt as expected (previously the fresh config silently shipped `recipients: []` and push would write plaintext)
- added default `.aisyncignore` and `.aisyncencrypt` scaffolding to `aisync init` — fresh repos start with safe basename-ignore patterns and a broad default encrypt pattern set covering memories, local settings, private keys (`*.key`, `*.pem`, `*.p12`, `*.pfx`, `*.jks`, `id_rsa`, `id_ed25519`, GPG keyrings), credential files (`.netrc`, `.pypirc`, `.npmrc`, `.dockerconfigjson`, `credentials*`, `auth.json`, `*.token`, `*.credentials`), env files, and session/cookie state
- added legacy-repo upgrade path in `aisync init` clone mode: missing `.aisyncignore`/`.aisyncencrypt` files are backfilled with defaults while existing user-customized content is left untouched
- added loud legacy-file warning on `aisync push` (and `--dry-run`): any file already in `personal/<tool>/` whose tool-relative path is no longer allowlisted triggers a `WARN` listing each obsolete path plus the exact `git -C <repo> rm -r ...` command to remove them. Non-destructive — push never deletes, it only reports — so users upgrading from the deny-list era are guided to clean up leftover `projects/**`, `paste-cache/**`, etc. on their own schedule
- added the `aisync nda` command group: `add`, `remove`, `list [--show]`, and `ignore`. The explicit forbidden list lives encrypted at `<repo>/.aisync-forbidden.age` (age-encrypted to the same recipients as the rest of the sync repo), so it travels between devices via the normal git flow with no extra cross-device handoff beyond the existing age key. Default matching is canonical-form substring (lowercase + NFKD-stripped + alphanumeric-only), so a single entry like `ZestSecurity` automatically catches `Zest Security`, `zest-security`, `ZEST_SECURITY`, `Zest.Security`, and `zést-sécurity` without the user enumerating variants. The `--word` flag adds word-boundary enforcement for short or generic terms; the `--regex` flag accepts a raw Go regex for power users

### Changed

- **BREAKING CHANGE:** replaced the compiled-in deny-list with a per-tool allowlist. aisync no longer tries to enumerate every new runtime/cache/transcript directory each AI vendor adds; instead, each known tool (`claude`, `cursor`, `copilot`, `codex`) has a small explicit list of syncable paths (`rules/**`, `agents/**`, `commands/**`, `hooks/**`, `hooks.json`, `skills/**`, `memories/**`, `output-styles/**`, `settings.json`, `settings.local.json`, `CLAUDE.md`, `AGENTS.md`, and per-tool equivalents) and everything else is NOT synced. Tools that lack a compiled-in entry fall back to a conservative default allowlist covering common cross-vendor conventions (`rules/**`, `agents/**`, `commands/**`, `skills/**`, `instructions/**`, `memories/**`, `settings*.json`). Users who were syncing paths outside this list (plugins, plans, custom subdirs) must add them to `tools.<name>.extra_allowlist` in `config.yaml`. The allowlist uses a strict matcher that does NOT fall back to basename matching, so a pattern like `CLAUDE.md` matches only the file at the tool root, not any file named `CLAUDE.md` deeper in the tree
- changed `aisync init` create mode to include **only detected (installed) tools** in the fresh `config.yaml`; tools that are not present on the device are omitted entirely rather than shipped as `enabled: false` placeholders. To enable additional tools later, add them to `config.yaml` by hand or re-run `aisync init` on a machine where they are installed
- changed the Go version to `1.26.2` and updated all module dependencies
- extended the `.aisyncignore` and `.aisyncencrypt` matcher to understand trailing-slash directory patterns (e.g. `plans/`) using the same contiguous-segment semantics as the compiled deny-list, so user ignore/encrypt files can cleanly exclude or mark whole directory trees
- pinned `aisync init` (create mode) to initialize the local Git repository on branch `main` regardless of the system's `init.defaultBranch` setting, so the fresh repo, `sync.branch` in `config.yaml`, and the assumed remote default always agree from the first commit
- refactored `aisync init` (create mode) to save `config.yaml` exactly once, after the age identity and recipient list are populated in memory; eliminates the interrupt window where a partial init could land the repo with `recipients: []` on disk and silently push plaintext secrets on the next `aisync push`
- simplified `aisync init` directory scaffolding to create only `personal/`, `shared/`, and `.aisync/` at the repo root; tool subdirectories (e.g. `personal/claude/rules/`) now emerge organically from `push`/`pull` as tools are detected, so fresh repos are no longer polluted with empty placeholder folders for AI tools the user does not actually use
- tightened the NDA auto-derivation filter so a fresh `aisync push --dry-run` produces meaningful findings instead of 100+ noise hits. `DirectoryLayout` now skips dotfile directories (`.idea`, `.vscode`, `.claude`, `.git`, `.github`, `.devcontainer`) under every forge root since they are universal IDE/tooling markers, never company identifiers. `addDerived` now drops canonical-form matches against a compile-time stop list of generic project-layout names (`backend`, `frontend`, `common`, `shared`, `core`, `src`, `lib`, `libs`, `docs`, `test`, `tests`, `internal`, `public`, `private`, `app`, `apps`, `api`, `apis`, `www`, `web`, `mobile`), URL-path conventions (`v1`, `v2`, `v3`, `v4`), branch names (`main`, `master`, `develop`), and AI-tool markers (`claude`, `cursor`, `vscode`, `idea`). Before this fix, a developer with `~/Development/dev.azure.com/<org>/backend/` on disk would see every English sentence containing the word "backend" in synced rule/command/agent files fire the NDA scanner — 100% false-positive rate, rendering the scanner unusable without aggressive per-device `nda.auto_derive_exclude` tuning. Users who genuinely need any of these as a forbidden term can still add it explicitly via `aisync nda add <term>` — the explicit list is checked independently and wins over the stop list
- upgraded the `.aisyncignore`/`.aisyncencrypt` matcher to support gitwildmatch-style `**` across path separators, so patterns like `personal/**/memories/**` match nested paths (`personal/claude/memories/nested/user.md`) the same way `.gitattributes` does — the tool and the Git clean/smudge filter can no longer disagree on recursive wildcards and silently leak plaintext from deep directory trees

### Fixed

- fixed `.aisyncencrypt` path matching in `push` so patterns like `personal/*/memories/**` and `personal/*/settings.local.json` actually match during dry-run, real push, and secret scan (previously matched against tool-relative paths, causing silently plaintext commits of content that should have been encrypted)
- fixed `MockConfigRepository.Load` to return a zero-value `*entities.Config` when the mock's `Config` field is nil. Purely a test-double convenience (not a mirror of production semantics — the real `YAMLConfigRepository.Load` wraps the underlying `os.ReadFile` error and returns `(nil, wrapped-err)` for missing files). Prevents a latent nil-pointer-deref foot-gun where a future test that forgot to set `Config` would crash the caller with a nil deref instead of getting a clean default config; tests that need to exercise the real missing-file error path should set `LoadErr` explicitly
- fixed a secret-scanner bypass where `scanForSecrets` skipped every file whose path matched an encrypt pattern, even when `config.Encryption.Recipients` was empty and `copyPersonalFile` had therefore written the file as plaintext (reachable via `aisync init` clone without `--key` import, or a stale `recipients: []` config). The scanner now mirrors the recipients gate from `copyPersonalFile`, so pattern-matched plaintext files are still scanned for leaked secrets. `copyPersonalFile` also logs a loud warning when a pattern matches but no recipients are configured, so operators notice the misconfiguration instead of silently committing plaintext
- fixed a slice-aliasing foot-gun in the NDA auto-deriver's `applyExcludes`: the exclude filter previously reused the input slice's backing array via `terms[:0:len(terms)]`, which was safe for current callers but would silently mutate the input for any future caller that kept its own reference to `terms`. Now allocates a fresh slice bounded at `len(terms)` capacity
- tightened NDA auto-derivation test coverage with a cross-source canonical-form dedupe case, a strict boundary-only adversarial case for the `ado-org-url` heuristic (`xhttps://dev.azure.com/CorporateOrg` — the inner pattern is otherwise valid so only the `(?:^|[^A-Za-z0-9.])` anchor can reject it), and end-to-end tests for `ExecGitInspector.EmailDomain` covering the public-free-mail allowlist (gmail/outlook), case-folding, malformed emails, and unset `user.email`

### Security

- closed the content-leak class that path allowlists, `.aisyncencrypt`, and credential regexes all miss by design: plaintext company names, project codenames, customer environment paths, and ADO/GitHub org URLs that legitimately live inside an allowlisted, unencrypted file (a `claude/rules/*.md` rule, an `agents/*.md` agent definition, etc.). The new NDA scanner runs on every push and blocks if any unencrypted file contains a forbidden term from the explicit list, the auto-derived machine-state list, or the compile-time heuristic shape checks. Findings are reported with the source tag (`user`, `auto-derived:<origin>`, `heuristic:<name>`) so the user can see exactly which knob fixes each hit, and the `aisync nda ignore <term>` workflow exists to silence specific false positives without disabling the whole pipeline
- closed the silent-leak class where every new subdirectory a vendor adds under `~/.claude/`, `~/.cursor/`, `~/.codex/`, etc. became syncable by default until someone noticed and filed a deny-list patch. The immediate trigger was `claude/paste-cache/` (a brand-new Claude Code runtime directory that the deny-list had not been updated for), which was about to leak the raw text of every recent paste — including full conversation context — into the public sync repo. With the new per-tool allowlist, unknown content is never synced, period. The failure mode flips from "silent plaintext leak when a vendor ships a new feature" to "loud skip of something unusual"
- pull and diff flows now refuse to apply a file from an external source to a tool directory unless the file's tool-relative path is in that tool's allowlist, logging a warning when a source tries to deliver an out-of-bounds path. A rogue or misconfigured source can no longer drop content into `.claude/projects/`, `.cursor/chats/`, or any other path the allowlist does not cover
- replaced the compiled-in deny-list with per-tool allowlist enforcement so only explicitly approved tool-relative paths are synchronizable by default. Claude/Cursor/Codex conversation transcripts, runtime state, backups, shell snapshots, file snapshots, IDE state files, and other non-allowlisted content such as `.claude/projects/`, `.claude/paste-cache/`, `.claude/history.jsonl`, `.cursor/chats/`, `.cursor/mcp.json`, `.codex/sessions/`, and `.aisync/state.json` are now blocked unless a user explicitly opts in via that tool's `extra_allowlist`
- the watch service now filters fsnotify and polling events through the per-tool allowlist instead of the old deny-list, so auto-push on file change can no longer stream a new runtime directory into the repo before the user notices. `WatchService.Watch` now takes a list of `WatchedTree` structs carrying the tool name and `extra_allowlist` per tree

## [0.1.0] - 2026-04-14

### Added

- added `--from-url` flag on `aisync source add` to import source definitions from YAML URLs
- added `--path` flag on `aisync source add` to restrict mappings to a subdirectory
- added `--polling-interval` flag on `aisync watch` for configuring file change detection interval
- added `--use-system-git` flag for environments where `go-git` has compatibility issues
- added `.gitattributes` creation with LF line ending enforcement and encryption filter patterns
- added `aisync device list/rename/remove` commands for managing registered devices
- added `aisync diff` command with summary/detailed modes, reverse mode, and external tool support
- added `aisync doctor` command with 7 diagnostic checks including Git connectivity
- added `aisync init` command to create or clone an `aifiles` repository
- added `aisync key generate/import/export/add-recipient` commands for `age` encryption management
- added `aisync migrate` command for legacy setup migration
- added `aisync pull` command to fetch from external sources and apply to AI tool directories
- added `aisync push` command with personal file detection, secret scanning, and dry-run mode
- added `aisync source add/remove/list/update/pin` commands to manage external sources
- added `aisync status` command to show sync state, source freshness, and offline indicator
- added `aisync sync` command combining pull and push in a single workflow
- added `aisync version` and `aisync self-update` commands
- added `aisync watch` command with `fsnotify`/polling dual-mode and auto-push debounce
- added `bubbletea` interactive diff viewer with keyboard scrolling
- added `gh repo create` suggestion in `aisync init` create flow
- added automatic version check on CLI startup using `CheckForUpdates()`
- added compiled-in deny-list for credentials, session transcripts, and plugin caches
- added CRLF-to-LF line ending normalization in atomic apply with binary file detection
- added cross-source file conflict detection and warning in `aisync source update`
- added force-push detection with user confirmation prompt
- added git clean/smudge filters for transparent `age` encryption (`_clean`/`_smudge` subcommands)
- added interactive TUI prompts via `charmbracelet/huh` with non-interactive fallback
- added manifest file (`.aisync-manifest.json`) for provenance tracking and deletion detection
- added offline connectivity indicator to `aisync status` output
- added per-file confirmation prompts during pull
- added recency warning when local files differ from incoming changes
- added shared/personal namespace separation with file-level precedence
- added tarball-only external source fetching with HTTP `ETag` and `Last-Modified` caching (zero API calls)
- added Tier 1 AI tool detection (Claude Code, Cursor, GitHub Copilot, Codex, Gemini CLI, Windsurf)
- added tool detection during `aisync init` clone workflow
- added Windows `%APPDATA%` config path resolution and `%ENVVAR%` expansion

### Changed

- changed `aisync diff` dry-run output to use KB/MB formatting and show line count deltas
- changed `aisync init` to parse `config.yaml` for encryption identity in clean/smudge filters

### Fixed

- fixed deny-list patterns: `.claude/.oauth` now uses trailing wildcard `.claude/.oauth*`
- fixed deny-list patterns: `.claude/projects/*/session` now uses trailing wildcard `.claude/projects/*/session*`
- fixed deny-list wildcard matching to support multiple `*` segments in a single pattern
