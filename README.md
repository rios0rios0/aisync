<h1 align="center">aisync</h1>
<p align="center">
    <a href="https://github.com/rios0rios0/aisync/releases/latest">
        <img src="https://img.shields.io/github/release/rios0rios0/aisync.svg?style=for-the-badge&logo=github" alt="Latest Release"/></a>
    <a href="https://github.com/rios0rios0/aisync/blob/main/LICENSE">
        <img src="https://img.shields.io/github/license/rios0rios0/aisync.svg?style=for-the-badge&logo=github" alt="License"/></a>
    <a href="https://github.com/rios0rios0/aisync/actions/workflows/default.yaml">
        <img src="https://img.shields.io/github/actions/workflow/status/rios0rios0/aisync/default.yaml?branch=main&style=for-the-badge&logo=github" alt="Build Status"/></a>
</p>

Sync AI coding assistant configurations across devices. Pull shared rules from community sources, push personal configs via Git, encrypt sensitive data with age.

## Features

- **30+ AI tools**: Claude Code, Cursor, Copilot, Codex, Gemini CLI, Windsurf, Cline, Roo Code, and [25+ more](#supported-ai-tools)
- **Cross-device sync**: Git-backed, no cloud dependency, no SaaS subscription
- **External sources**: Pull shared rules from any public Git repository (tarball-only, zero API calls)
- **Shared/personal split**: Enforced namespaces — shared rules from sources, personal rules across devices
- **Age encryption**: Memories and sensitive settings encrypted at rest — public repos are safe
- **File watching**: Real-time change detection with fsnotify + polling fallback for Termux
- **Diff preview**: See what would change before applying, with recency detection
- **Secret scanning**: 15 regex patterns block accidental credential leaks before push
- **Atomic apply**: Two-phase commit with journal prevents partial updates
- **Self-update**: Single binary, updates from GitHub Releases via `aisync self-update`

## Quick Start

```bash
# Install
go install github.com/rios0rios0/aisync/cmd/aisync@latest

# Initialize a new aifiles repo
aisync init

# Generate an age encryption key
aisync key generate

# Add the engineering guide as an external source
aisync source add guide --source-repo rios0rios0/guide --branch generated

# Pull shared rules into ~/.claude/, ~/.cursor/, ~/.codex/, etc.
aisync pull

# Check status
aisync status
```

### Restore on a new device

```bash
# Clone your existing aifiles repo (convention: <github-user>/aifiles)
aisync init rios0rios0

# Or with a full URL
aisync init --remote-url git@github.com:rios0rios0/aifiles.git --key ~/.config/aisync/key.txt
```

### Upgrade scaffolding on an existing repo

If your aifiles repo was initialized before recent template changes (for example, before the comprehensive `.aisyncencrypt` patterns or the `**/.gitignore` exclusion in `.aisyncignore` were added), refresh the scaffolding files in place:

```bash
# Overwrites .gitignore, .aisyncignore, and .aisyncencrypt with current templates.
# config.yaml, repo content, and Git state are left untouched.
aisync init --refresh-scaffolding
```

Local edits to those three files are overwritten — review the changes with `git diff` before committing. Custom additions should be saved externally and reapplied after the refresh.

## The `aifiles` Convention

Like chezmoi expects a `dotfiles` repo, aisync expects an `aifiles` repo:

| Aspect | chezmoi | aisync |
|--------|---------|--------|
| Repo name | `dotfiles` | `aifiles` |
| Local clone | `~/.local/share/chezmoi/` | `~/.config/aisync/repo/` |
| Shorthand | `chezmoi init <user>` | `aisync init <user>` |

The `aifiles` repo can be **public** — personal data (memories, local settings) is encrypted with age.

## Usage

### Daily Workflow

```bash
aisync sync          # Pull shared rules + push personal changes
aisync watch         # Monitor for changes in real-time
aisync watch --auto-push  # Auto-push after 60s debounce
```

### Source Management

```bash
aisync source add guide --source-repo rios0rios0/guide --branch generated
aisync source add ecc --source-repo affaan-m/everything-claude-code --branch main
aisync source pin ecc --ref v2.1.0
aisync source list
aisync source update
aisync source remove ecc
```

### Encryption

```bash
aisync key generate          # Create age key pair
aisync key export            # Print public key
aisync key add-recipient <pubkey>  # Multi-device decryption
aisync key import <path>     # Import existing key
```

### Diagnostics

```bash
aisync status     # Sync state, managed files, sources (offline indicator if unreachable)
aisync diff       # Preview what would change (interactive TUI viewer when TTY)
aisync doctor     # Check config, git, age key, tools, sources, git connectivity
aisync device list
```

### Advanced Options

```bash
# Use system git instead of built-in go-git (for Git LFS, SSH edge cases)
aisync pull --use-system-git

# Add source from a specific subdirectory
aisync source add mytools --source-repo org/repo --branch main --path tools/claude

# Import source definition from a URL
aisync source add --from-url https://example.com/aisync-source.yaml

# Configure polling interval for Android/Termux
aisync watch --polling-interval 15s --auto-push
```

### Encryption Filters

New repos created with `aisync init` are automatically configured with git clean/smudge filters for transparent age encryption. Files matching `.aisyncencrypt` patterns are encrypted on `git add` and decrypted on `git checkout`. This works with `--use-system-git`; with built-in go-git, encryption is handled during push/pull.

## Recommended External Sources

| Source | Repository | Branch | Description |
|--------|-----------|--------|-------------|
| **Guide** | `rios0rios0/guide` | `generated` | 14 engineering standard rules, 7 agents, 8 commands, 5 skills |
| **Agents** | `wshobson/agents` | `main` | 112+ specialized agents (docs, TDD, CI/CD) |
| **Everything Claude Code** | `affaan-m/everything-claude-code` | `main` | Anthropic Hackathon winner. 28 agents, 125 skills, 60 commands |
| **Power Platform** | `microsoft/power-platform-skills` | `main` | Official Microsoft Power Platform plugins |
| **HVE Core** | `microsoft/hve-core` | `main` | 49 agents, 102 instructions, 63 prompts, 11 skills |

```bash
aisync source add guide --source-repo rios0rios0/guide --branch generated
aisync source add agents --source-repo wshobson/agents --branch main
aisync source add ecc --source-repo affaan-m/everything-claude-code --branch main
aisync source add power-platform --source-repo microsoft/power-platform-skills --branch main
aisync source add hve-core --source-repo microsoft/hve-core --branch main
aisync pull
```

## Supported AI Tools

### Tier 1 (full support)

Claude Code, Cursor, GitHub Copilot (IDE + CLI), Codex CLI, Gemini CLI, Windsurf

### Tier 2 (extended support)

Cline, Roo Code, Continue.dev, Aider, Zed, Trae, Warp, Amazon Q Developer, Amp, Junie, Kilo Code, Goose

### Tier 3 (community support)

OpenCode, OpenClaw, Antigravity, Kiro, Factory/Droid, Augment Code, Tabnine, Qwen Code, Rovo Dev, deepagents, Replit, Blackbox AI, JetBrains AI

New tools are added by editing `config.yaml` — aisync treats all files as opaque blobs.

## How aisync Compares

| Feature | **aisync** | memoir.sh | memories.sh | claude-brain | chezmoi | ai-rules-sync | skillshare | rulesync |
|---------|-----------|-----------|-------------|--------------|---------|---------------|------------|----------|
| **Open source (fully)** | Yes (MIT) | CLI only | CLI only | Yes (MIT) | Yes (MIT) | Yes | Yes (MIT) | Yes (MIT) |
| **No cloud dependency** | Yes (Git) | No ($15-29/mo) | No ($15-25/mo) | Yes (Git) | Yes (Git) | Yes (Git) | Yes | Yes |
| **AI tools supported** | 30+ | 11 | 19 | 1 | N/A | 31 | 55+ | 24 |
| **Cross-device sync** | Yes | Yes (paid) | Yes (paid) | Yes | Yes | No | No | No |
| **External source pull** | Yes | No | No | No | Yes | Yes | Yes | Yes |
| **Encryption (at rest)** | Yes (age) | Yes (AES) | Yes | Yes (age) | Yes | No | No | No |
| **File watching** | Yes | No | No | No | No | No | No | No |
| **Diff preview** | Yes | Yes | Yes | No | Yes | No | No | No |
| **Secret scanning** | Yes | Yes | No | No | No | No | Yes | No |
| **Shared/personal split** | Yes | Profiles (paid) | Profiles (paid) | Yes | Manual | Yes | No | No |
| **Scoped to AI tools** | Yes | Yes | Yes | Yes | No | Yes | Yes | Yes |
| **Atomic apply** | Yes (journal) | No | No | No | No | No | No | No |
| **Offline support** | Yes | No | No | Yes | Yes | N/A | N/A | N/A |

### When to Use Each Tool

| If you need... | Use |
|----------------|-----|
| Sync AI configs across devices + pull shared rules | **aisync** |
| MCP-based persistent memory with cloud backup | **memoir.sh** or **memories.sh** |
| Semantic merge of Claude Code memory using LLM | **claude-brain** |
| General-purpose dotfile management | **chezmoi** |
| Distribute rules on the same machine via symlinks | **ai-rules-sync** or **skillshare** |
| Generate tool-specific rules from a unified source | **rulesync** |
| Claude Code native plugin distribution | **Claude Code Plugin Marketplace** |

aisync is **complementary** to the Claude Code Plugin Marketplace — plugins cannot distribute rules (`.claude/rules/*.md`), which is aisync's core value.

## Why Not chezmoi?

aisync is heavily inspired by chezmoi — Git-backed sync, age encryption, diff previews, external source fetching. But chezmoi solves a **different problem**.

| Concern | chezmoi | aisync |
|---------|---------|--------|
| **Scope** | Entire `$HOME` | Only AI tool directories |
| **Shared vs personal** | Manual (no enforcement) | Enforced namespaces (`shared/`, `personal/`) |
| **Multi-source aggregation** | `.chezmoiexternal.toml` (fragile at scale) | First-class with ordering, conflict detection, provenance |
| **Merge strategies** | Generic 3-way text merge | AI-tool-aware (hooks concat, settings deep merge, section concat) |
| **File watching** | Declined ([issue #752](https://github.com/twpayne/chezmoi/issues/752)) | Built-in (fsnotify + polling fallback) |
| **User prerequisite** | Full chezmoi installation | Single binary, one config file |

**Use chezmoi for your dotfiles. Use aisync for your AI tools.** Add `~/.claude/`, `~/.cursor/`, `~/.codex/`, `~/.gemini/` to `.chezmoiignore` and let aisync manage those directories.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

[MIT](LICENSE)
