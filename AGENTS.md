# Repository Guidelines

## Project Structure & Module Organization
- Root Go module in `go.mod`. Single binary in `cmd/cord` with two top-level subcommands:
  - `cord server` (`cmd/cord/server.go`): coordination server commands.
  - `cord client` (`cmd/cord/client.go`): client/administration commands.
- Shared packages in `internal/`: `server`, `client`, `database`, `wireguard`, `utils`.
- Build outputs in `bin/`. Docs in `docs/` (ADRs in `docs/adrs/`).
- Tests are colocated as `*_test.go` next to sources (e.g., `internal/server/server_test.go`).

## Build, Test, and Development Commands
- `make all` (or `make cord`): builds `./bin/cord` from `cmd/cord`.
- `make test` or `go test ./...`: runs the unit tests.
- `sudo make test-integration`: integration tests that create real WireGuard interfaces.
- Example: `./bin/cord server add-network <name> <cidr> <external-ip> <port>`.
- Dev tip: override paths with `--config-dir` and `--data-dir`.

## Coding Style & Naming Conventions
- Go 1.24+. Always format with `gofmt`/`goimports` before pushing.
- Always use **tabs** for leading indentation.
- Packages are small and singular (`server`, not `servers`). Exported identifiers follow Go idioms.
- Errors: wrap with context (`fmt.Errorf("op: %w", err)`); avoid panics in libraries.
- CLI: subcommands use kebab-case. Layout: entrypoints in `cmd/<name>/main.go`; logic in `internal/<pkg>`.

## Testing Guidelines
- Framework: standard Go `testing` (prefer table-driven tests).
- Naming: `Test<Type_Action>`; helpers unexported.
- Run a package: `go test ./internal/server -v`; full suite: `go test ./...`.
- Add cases for CIDR math, associations, and invite/peer lifecycle; aim for meaningful coverage without flakiness.

## Commit & Pull Request Guidelines
- Commits: small, imperative subjects; prefix with area/keyword when useful
  (e.g., `server: validate CIDR containment` or `added: update peer flow`).
- Body: explain what/why, note trade-offs; link ADRs/issues.
- PRs: clear description, CLI output screenshots/logs if behavior changes; call out migrations or config path changes. Include tests and doc updates (`README.md`, `docs/adrs`).

## Security & Configuration Tips
- Never commit WireGuard keys, invite payloads, or real endpoints.
- Prefer temp dirs in development via `--config-dir`/`--data-dir`.
- SQLite schema is owned by the server; keep test DBs within the project, not system paths.

## Architecture Overview
- One CLI over `internal/*`. Server persists state in SQLite; `wireguard` handles key generation and config serialization. Keep package boundaries clean and cyclic-dep free.
