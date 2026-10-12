# DNS and runtime follow-ups

Ideas for improving runtime behavior and preserving clear ownership as networks,
devices, and DNS resources start, fail, and shut down.

## Independent progress for each network

The runtimes currently execute commands and maintenance sequentially. A slow
device operation can delay other networks and status reads. If that becomes a
problem, each network could have its own worker so unrelated networks keep
making progress:

```text
Network A: reconciliation blocks on a device operation
Network B: reconciliation and cleanup still complete
Status: snapshots remain available while A is blocked
```

Work for one network should still be sequential. Repeated wake requests could
share a pending pass, and queued work should use the latest persisted intent.
Cancellation should resolve waiting callers and wait for workers to finish;
unexpected worker exits should reach the runtime owner.

Independent workers would add coordination around status snapshots, cleanup,
replacement, and command completion. The simpler sequential model may be enough
unless slow operations cause observable interference.

## Treat database corruption as a process-level failure

Database corruption could trigger an orderly daemon exit rather than repeated
retries. SQLite corruption or a non-database file is different from a temporary
network or listener failure:

```text
Peer read -> SQLite reports NOTADB -> daemon exits with the cause
DNS bind -> temporary port conflict -> resource remains retryable
```

There is no explicit corruption-to-daemon path today. If added, corruption
detected after startup should reach the process owner with the original SQLite
cause intact. The daemon should cancel owned work, collect shutdown errors, and
release resources before closing the database. Ordinary operation errors should
remain recoverable.

This should fit the existing retry, restart, and fatal error model. Test the
behavior through public operations over real SQLite where practical.

## Make device recovery decisions explicit

Owners should distinguish an operation that can be retried from a device that
needs replacement. A transient peer update failure should not rebuild a healthy
interface. If the main interface disappears, its dependent HTTP and DNS
listeners should stop before the interface is recreated.

Device reconciliation and recovery classification already exist. Further changes
should classify failures at the operation's boundary, preserve the underlying
cause, and restore the device before applying peers.

## Preserve ownership through incomplete cleanup

A resource still belongs to its owner until cleanup finishes. Failed shutdown
should not make that resource disappear from tracking or let a replacement
bypass it. Much of this is already covered by the resource model; future changes
should preserve these expectations:

- Failed cleanup retains ownership and remains visible and retryable.
- Re-enable or configuration replacement cannot bypass unfinished cleanup.
- DNS and HTTP consumers stop before their dependent device is removed.
- Background failure is reported, and shutdown joins owned work.
