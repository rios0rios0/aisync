# Contributing

Contributions are welcome. By participating, you agree to maintain a respectful and constructive environment.

For coding standards, testing patterns, architecture guidelines, commit conventions, and all
development practices, refer to the **[Development Guide](https://github.com/rios0rios0/guide/wiki)**.

## Prerequisites

- Go 1.26+
- [Make](https://www.gnu.org/software/make/)
- [chlog](https://github.com/luizjhonata/chlog) (`go install github.com/luizjhonata/chlog@latest`)

## Development Workflow

1. Fork and clone the repository
2. Create a branch: `git checkout -b feat/my-change`
3. Install dependencies:
   ```bash
   go mod download
   ```
4. Make your changes
5. Validate:
   ```bash
   make build
   make test
   ```
6. Add a changelog fragment — never edit `CHANGELOG.md`, which is generated from them:
   ```bash
   chlog new --kind Added --body "added the thing that was not there before"
   ```
7. Commit following the [commit conventions](https://github.com/rios0rios0/guide/wiki/Git-Flow)
8. Open a pull request against `main`

## Architecture

The project follows Clean Architecture:

```
aisync/
├── cmd/aisync/           # Entry point
├── internal/
│   ├── domain/           # Contracts (no external dependencies)
│   │   ├── commands/     # Business logic
│   │   ├── entities/     # Core types
│   │   └── repositories/ # Interfaces
│   └── infrastructure/   # Implementations
│       ├── controllers/  # CLI (cobra)
│       ├── repositories/ # Config, git, manifest, state, journal
│       └── services/     # Encryption, diff, watch, merge, scanning
└── test/                 # Test builders and doubles
```

Dependencies always point inward: infrastructure depends on domain, never the reverse.
