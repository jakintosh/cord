# Testing

Tests should make the project's observable guarantees easy to read. Test public
behavior by default. When persistence matters, prefer the real SQLite adapter
over mock stores. Test pure computations directly without constructing an
application environment. A test should show its starting conditions, operation,
expected result, and any important durable effect.

## Structure and style

- Use an external `package_test` package by default.
- Put tests in the matching source file’s `*_test.go`. A public facade may drive
  behavior implemented by that source file; it does not require internal access.
  Do not add qualifiers such as `example_widgets_test.go` for `example.go`.
- Reserve `*_internal_test.go` for a small, justified mechanical invariant that
  would need a more complicated integration harness to exercise.
- Name tests `TestOperation_Behavior`; match production multiline signatures.
- Keep helpers in the corresponding test files and use `t.Helper()`.
- Use tables for input permutations with the same setup and assertions. Split
  lifecycle scenarios with different control flow into separate tests.
- Assert meaningful values, sentinel errors, and unchanged state after rejected
  mutations. Avoid assertions that merely repeat implementation steps.

## Shared foundations

Use the package group's existing `testutil` package as the composition root,
such as `internal/client/testutil` or `internal/server/testutil`. Prefer a small
number of shared environments and baseline fixtures over bespoke setup in each
test. Choose the smallest environment that exercises the behavior:

| Subject | Baseline |
| --- | --- |
| Persistence | Isolated SQLite database |
| Domain behavior | Service over real SQLite |
| Runtime behavior | Service and runtime with the existing backend fixtures |
| HTTP contracts | In-process router over the shared environment |
| Remote interactions | Existing server-backed variants when needed |

Each environment owns isolated state and cleanup through `t.Cleanup()`. Share
fixture definitions, not mutable database state between tests. Use shared seed
helpers for established state, but call the actual operation when its validation
or lifecycle behavior is the subject. Keep unusual inputs and important
relationships visible in the test instead of burying them in a universal fixture.

Shared setup owns cleanup in dependency order: stop consumers before closing
their dependencies. For example, stop a runtime before closing its remote HTTP
server, then close the database. Consumers should not duplicate that wiring or
register redundant cleanup. Use parallel tests only when their state and
resources are isolated.

## Coverage by boundary

- **Database:** public adapter operations, durable round trips, constraints,
  rejected-write rollback, retries, ordering, and file reopen. Use SQL only where
  schema or transaction mechanics cannot be observed through store methods.
- **Service:** validation, lifecycle rules, protocol-to-domain conversion, and
  public errors over real SQLite. Avoid repeating every adapter detail.
- **Runtime:** intent versus observed state, resource ownership, recovery,
  cancellation, and external effects using the existing test backend.
  Keep narrow fault fixtures only when they prove a distinct lifecycle guarantee.
- **API:** in-process router requests, explicit URLs/bodies/headers, status before
  typed payloads, error envelopes, and meaningful side effects. Use short phase
  comments to separate setup, request, result, and state checks.
- **Daemon:** one real composition smoke test and simple startup failures. Leave
  socket internals to the daemon package.
- **Test utilities:** rely primarily on consumers; test only reusable contracts
  whose failure would undermine many suites.
- **Other packages:** test public contracts directly, with tables for input
  permutations. Add integration setup only when the behavior depends on it.

Production code must not acquire interfaces, exported internals, or behavior
solely to support tests. Record inaccessible fault cases instead of constructing
a parallel subsystem to reach them. Statement coverage is diagnostic, not a
target.

Explain each white-box exception briefly beside the test. A deterministic
selection algorithm or a complicated resource-ownership invariant may warrant
direct testing when an integration harness would obscure the behavior. Internal
visibility alone is not a reason to add a test.

## Validation

Run `make test-full` for the repository suite with the race detector. Use the
normal Go cache, keep temporary artifacts under `.opencode-tmp`, and never add
Makefile targets merely for the tests. Verify that existing non-test Go files are
unchanged before completing a test-only refactor.
