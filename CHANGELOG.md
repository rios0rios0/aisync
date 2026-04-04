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

### Added
- added `aisync init` command to create or clone an `aifiles` repository
- added `aisync source add/remove/list/update` commands to manage external sources
- added `aisync pull` command to fetch from external sources and apply to AI tool directories
- added `aisync status` command to show sync state and source freshness
- added `aisync version` and `aisync self-update` commands
- added Tier 1 AI tool detection (Claude Code, Cursor, GitHub Copilot, Codex, Gemini CLI, Windsurf)
- added manifest file (`.aisync-manifest.json`) for provenance tracking and deletion detection
- added tarball-only external source fetching with HTTP ETag caching (zero API calls)
- added shared/personal namespace separation with file-level precedence
- added compiled-in deny-list for credentials, session transcripts, and plugin caches

### Changed

### Removed
