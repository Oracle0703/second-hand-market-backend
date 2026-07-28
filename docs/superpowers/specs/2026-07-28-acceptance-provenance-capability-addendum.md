# Acceptance Provenance Capability Addendum

**Date:** 2026-07-28

**Parent design:** `2026-07-28-acceptance-provenance-hardening-design.md`

**Status:** Unified Approach A architecture and written specification were
approved on 2026-07-28. This addendum records the implementation ruling needed
to apply that approved fail-closed architecture to the final review's
path-replacement findings. It does not authorize
SSH, transfer, remote execution, Docker-server access, or production access.

## 1. Scope

This addendum applies only to the four provenance harnesses and their contract
tests. It does not change application behavior, migration behavior, evidence
schemas, package whitelists, or server authority. Runtime code remains
Bash 3.2-compatible and duplicated per harness; no shared runtime helper or new
toolchain dependency is introduced.

## 2. Approaches Considered

### 2.1 Adopted: Bash 3.2 capability-relative lock commit

Use immediate cwd binding for acquired directories, private descriptor-backed
configuration snapshots, and an owned publication lock as the commit marker.
This closes the concrete traversal, deletion, mutable-input, and ambiguity
failures without changing package contents or adding a runtime dependency.

### 2.2 Rejected: shared native `openat`/`renameat2` helper

A native helper could provide stronger descriptor primitives on Linux, but it
would add a fifth executable trust boundary, change every package whitelist,
and fail the current macOS/Bash 3.2 portability contract. It also contradicts
the approved decision to keep each harness independently reviewable.

### 2.3 Rejected: keep staging rename and document residual races

Retaining `mv -n` cannot meet the contract. On macOS an existing destination
directory may receive the source as a nested child, and a signal or nonzero
result after rename leaves the caller unable to know whether publication
occurred. A warning in documentation would not prevent unsafe cleanup or a
false successful audit.

## 3. Portable Capability Boundary

Portable Bash cannot provide `openat(2)`/`renameat2(2)` descriptor semantics.
The harnesses therefore make the following narrower, testable guarantee:

1. Enter an acquired directory immediately and bind its device/inode identity
   from `.`.
2. Perform later reads, writes, validation, and bounded cleanup relative to
   that bound current-working-directory capability.
3. Recheck the caller-visible path against the bound identity before reporting
   success.
4. Never recursively delete through a separately re-resolved absolute or child
   pathname. On identity ambiguity, retain state and fail closed.

The unavoidable `mkdir`-to-`cd` acquisition interval is not represented as an
OS descriptor guarantee. Tests must cover every replacement point after
acquisition, and documentation must not claim immunity from a privileged actor
that can replace paths inside that interval. The safety invariant is that an
ambiguous path cannot cause traversal-based deletion or a successful result.

## 4. Export Directory Protocol

The exporter enters the destination parent first, records the parent identity
from `.`, creates the absent basename relative to that parent, immediately
enters it, and records the child identity from `.`. All four package artifacts,
temporary validation data, and checks are capability-relative.

Failure cleanup removes only known exporter-created entries through the bound
child capability. It never runs recursive cleanup through the requested
destination pathname. After returning through `..`, an empty top-level entry
may be removed only while the parent and child identities still match; any
uncertainty retains the incomplete directory and returns failure. A later run
continues to reject that pre-existing destination.

## 5. Evidence Publication Protocol

The publication lock is the commit marker. While it exists, any final evidence
directory is incomplete or ambiguous and must not be audited or reused.

Publication proceeds under the bound evidence-parent capability:

1. Atomically acquire an absent lock directory with `mkdir`, immediately enter
   it, and bind its identity from `.`.
2. Atomically acquire the absent final evidence directory with `mkdir`; a
   regular file, directory, or symlink at either name fails closed.
3. Immediately enter and identity-bind the final directory, copy only the
   already sanitized candidate through `.`, then validate its schema, leak scan,
   and hashes in place.
4. Remove the owned empty lock only after final validation and identity checks.
   Lock removal is the publication commit point.

There is no staging-directory rename and no recursive cleanup through the final
name. Any signal, command failure, identity mismatch, or uncertain lock-removal
outcome retains both the final directory and lock. Normal-mode preflight rejects
either retained state before Docker or npm. This converts rename ambiguity into
an explicit fail-closed state.

## 6. Immutable Runtime Inputs

Docker Compose must consume the verified private build-context copy of
`deploy/acceptance/docker-compose.yml`, never the received-tree pathname.

Every Docker harness opens its received `.env` through two independent fixed
read-only file descriptors after type and identity checks. Both descriptors
must bind the same prevalidated regular-file device/inode. The harness records
each descriptor's identity, size, mode, modification time, and change time;
copies each descriptor independently into a separate mode-`0600` runtime
candidate; then rechecks both descriptor signatures. The two candidates must be
byte-identical. One validated candidate becomes the only `--env-file` consumed
by Compose and the other is removed. A path replacement, copy-time metadata
change, candidate mismatch, or descriptor mismatch fails before Docker.

This is a byte-stability check across two independently opened descriptions,
not a claim that portable Bash places a mandatory write lock on the source
inode. A peer with authority to rewrite the same inode before both reads is
outside the provenance guarantee; remote `.env` authenticity remains an
operator/secrets boundary. The harness guarantee is that it never follows a
later pathname replacement and never consumes a mixed or non-repeatable copy.

All Compose and MySQL standard error remains under the private runtime
directory. No received execution-configuration pathname is consumed after its
snapshot is accepted.

## 7. Required Regression Evidence

Behavioral tests must execute the real scripts with command wrappers and prove:

- export replacement during acquisition checks and cleanup cannot write to or
  recursively delete an external marker;
- evidence parent, lock, and final-name replacements retain external markers
  and cannot yield a successful publication;
- post-validation replacement of received Compose or `.env` files cannot alter
  Compose inputs;
- F-06 copy-time `.env` ABA/content mutation is refused before Docker;
- a signal or nonzero outcome during publication retains the final directory
  and lock, and preserves original `130`/`143` signal status.

The four complete harness suites, combined migration gate, backend full suite,
race suite, `go vet`, shell syntax, Go formatting, and diff/scope checks remain
mandatory before a code-side completion claim.
