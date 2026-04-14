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
- added Git connectivity check to `aisync doctor`
- added interactive TUI prompts via `charmbracelet/huh` with non-interactive fallback
- added manifest file (`.aisync-manifest.json`) for provenance tracking and deletion detection
- added offline connectivity indicator to `aisync status` output
- added per-file confirmation prompts during pull
- added recency warning when local files differ from incoming changes
- added shared/personal namespace separation with file-level precedence
- added tarball-only external source fetching with HTTP ETag and `Last-Modified` caching (zero API calls)
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

