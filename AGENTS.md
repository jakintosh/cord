# Repository Guidelines

## Build, Test, and Development Commands
- Use the `Makefile` for build, test, lint, and similar workflows. Run `make help` to see available targets.
- Never use temporary GOCACHE caches; ask for build permission if necessary, and use `make build`.
- Do not add new targets to the Makefile unless explicitly asked.

## Temporary Files
- Use a `.opencode-tmp/` directory in the project root for temporary files (build output, test sockets, etc.) instead of `/tmp/`. This keeps work contained in the project and avoids cross-device issues.
