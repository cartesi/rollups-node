# TLA+ Specification: Drain Protocol

Formal specification of the application drain protocol for the Cartesi
rollups-node. The drain protocol coordinates safe removal of an application
across multiple services that hold cross-tick state (machine processes,
in-flight L1 transactions).

## Overview

The specification models a system with:

- A **PostgreSQL database** (source of truth) storing application lifecycle
  state (`enabled`, `deleted_at`) and per-service drain acks.
- **Three stateful services** (advancer, claimer, PRT) that must acknowledge
  when they have finished all work for a soft-deleted application.
- An **operator** (CLI) who can enable/disable apps (reversible toggle),
  initiate drain via soft-delete (irreversible), and hard-delete via purge.
- **Non-deterministic failures**: service crashes that clear volatile state,
  and L1 transactions that can be submitted and confirmed at any time.

The key design invariant: **the database is the single source of truth** for
application lifecycle. On each tick, services scan the database for all
soft-deleted apps they are required for and ack them. No volatile in-memory
state is needed for drain detection.

### Application Lifecycle

Enable/disable is a **reversible operational toggle**. Disable stops processing
and frees resources (machines destroyed) but does NOT trigger drain. Disabled
apps can be re-enabled (machines rebuilt from snapshot) or remain as archives
indefinitely. Drain starts when the operator **soft-deletes** the app (sets
`deleted_at`). This is the irreversible "point of no return." Services detect
soft-delete and begin cleanup + ack. Stale acks are structurally impossible
because drain only triggers on soft-delete, which is irreversible, and
re-enable requires `~dbDeleted`.

### Design Decisions

| ID | Decision | Rationale |
|----|----------|-----------|
| D2 | Scan-on-every-tick for ack detection | Each tick acks ALL soft-deleted apps the service is required for. Crash-restart safe. Self-healing on failed writes. |
| D3 | Health state not modeled | `STOPPED`/`FAILED` are irrelevant to drain correctness. Simplifies the spec. |
| D4 | PRT apps with active tournaments require explicit override to disable/remove | Prevents accidental bond forfeiture. Modeled via `SafeToDisable` guard + `OperatorForceDisable`/`OperatorForceSoftDelete`. |

## Files

| File | Purpose |
|------|---------|
| `DrainProtocol.tla` | Main specification: 16 actions, 7 safety properties, 2 liveness properties |
| `MC.tla` | Model-checking wrapper with concrete constants (2 apps, 3 services) |
| `DrainProtocol.cfg` | TLC configuration |

## The Model

### Variables

| Variable | Type | Maps to in Go |
|----------|------|---------------|
| `dbEnabled` | `[AppIDs -> BOOLEAN]` | `application.enabled` column |
| `dbDeleted` | `[AppIDs -> BOOLEAN]` | `application.deleted_at IS NOT NULL` |
| `dbAcks` | `[AppIDs -> SUBSET ServiceNames]` | Rows in `application_service_ack` |
| `dbHardDeleted` | `SUBSET AppIDs` | Application row deleted from DB |
| `forceDeleted` | `SUBSET AppIDs` | Apps hard-deleted via force purge (tracking set) |
| `advMachines` | `SUBSET AppIDs` | `machineManager.machines` map (advancer) |
| `clmKnown` | `SUBSET AppIDs` | Active Authority/Quorum apps tracked by claimer |
| `clmInFlight` | `SUBSET AppIDs` | `claimsInFlight` map (claimer) |
| `prtKnown` | `SUBSET AppIDs` | Active PRT apps tracked by PRT service |
| `prtInFlight` | `SUBSET AppIDs` | `settleInFlight`/`joinInFlight` maps (PRT) |
| `svcAlive` | `[ServiceNames -> BOOLEAN]` | Service process liveness |

### Constants

| Constant | Purpose |
|----------|---------|
| `AppIDs` | Set of application IDs (e.g., `{1, 2}`) |
| `ServiceNames` | `{"advancer", "claimer", "prt"}` |
| `ConsensusType` | Function: app ID -> `"AUTHORITY"` or `"PRT"` |
| `PrtTournamentActive` | Function: app ID -> BOOLEAN (on-chain tournament state) |

### Actions

| TLA+ Action | Go Implementation | Description |
|-------------|-------------------|-------------|
| `OperatorDisable(a)` | `SetApplicationEnabled(ctx, a, false)` | Disable app (reversible; blocked if PRT + active tournament) |
| `OperatorForceDisable(a)` | `app disable --acknowledge-bond-loss` | Force-disable PRT app with active tournament |
| `OperatorReEnable(a)` | `SetApplicationEnabled(ctx, a, true)` | Re-enable disabled app (blocked if soft-deleted or INOPERABLE) |
| `OperatorSoftDelete(a)` | `app remove` | Atomically disable + set `deleted_at`; initiates drain |
| `OperatorForceSoftDelete(a)` | `app remove --acknowledge-bond-loss` | Force-remove enabled PRT app with active tournament |
| `OperatorPurge(a)` | `HardDeleteApplication(ctx, a)` | CASCADE delete after ack check |
| `OperatorForcePurge(a)` | `purge --force` CLI command | Hard-delete bypassing ack check |
| `AdvancerTick` | `Step()` in `advancer.go` | Scan soft-deleted apps, ack those advancer is required for |
| `ClaimerTick` | `Tick()` in `claimer/service.go` | Scan soft-deleted Authority apps, ack (defer if in-flight) |
| `PRTTick` | `Tick()` in `prt/service.go` | Scan soft-deleted PRT apps, ack (defer if in-flight) |
| `ClaimerSubmitClaim(a)` | L1 claim submission | Non-deterministic claim for active app |
| `ClaimerClaimConfirmed(a)` | L1 claim mined | Claim confirmed on-chain |
| `PRTSubmitTx(a)` | L1 settle/join submission | Non-deterministic PRT transaction |
| `PRTTxConfirmed(a)` | L1 tx mined | PRT transaction confirmed |
| `ServiceCrash(s)` | OOM kill, process death | Lose all volatile state |
| `ServiceRestart(s)` | Container restart | Volatile state empty; next tick scans DB |

### Properties

**Safety (checked as invariants):**

- `Safety_PurgeRequiresAcks` -- Hard delete only after all required acks
  (or via explicit force-purge).
- `Safety_AdvAckImpliesNoMachine` -- Advancer ack means no machine process.
- `Safety_ClmAckImpliesNoInFlight` -- Claimer ack means no pending L1 claim.
- `Safety_PRTAckImpliesNoInFlight` -- PRT ack means no pending L1 tx.
- `Safety_NoAcksBeforeDrain` -- Non-deleted apps have no ack rows. Drain
  acks are only written for soft-deleted apps, making stale acks from
  enable/disable cycling structurally impossible.
- `Safety_ForceImpliesHardDeleted` -- Force-deleted apps are a subset of
  hard-deleted apps (defensive regression check).
- `Safety_DeletedImpliesDisabled` -- Soft-deleted apps are necessarily
  disabled (holds by construction, regression check).

**Liveness (checked as temporal properties):**

- `Liveness_DrainCompletes` -- Every soft-deleted app is eventually fully
  acked or explicitly force-purged. Disabled-but-not-deleted apps have no
  liveness obligation (they can remain as archives indefinitely).
- `Liveness_ForcePurgeCleanup` -- After force-purge, services eventually
  clean up orphaned volatile state (machines, in-flight L1 transactions).

### Fairness

Service ticks use **strong fairness** (`SF`) because a crash temporarily
disables the tick action. SF guarantees: if enabled infinitely often, it
eventually fires. Same reasoning as the `HybridEvents` spec.

Service restarts and L1 confirmations use **weak fairness** (`WF`).

## TLC Results

```
Model checking completed. No error has been found.
  30,939 states generated, 4,072 distinct states found
  Depth: 13
  Time: <1 second
  7 safety invariants + 2 liveness properties verified
```

## Running the Model Checker

```bash
cd spec/drain-protocol/
java -jar ~/tla/tla2tools.jar \
  -config DrainProtocol.cfg \
  -workers auto \
  -metadir /tmp/tlc \
  MC.tla
```

Expected output: `Model checking completed. No error has been found.`

## Compositional Independence

The DrainProtocol spec models service ticks directly, abstracting away the
event delivery mechanism (specified in `HybridEvents`). This is valid because
the drain protocol's correctness depends only on ticks eventually happening
(SF fairness), not on how they are triggered. Events provide faster
notification but the protocol's correctness relies solely on the
scan-on-every-tick mechanism.

The drain protocol does not depend on the events library. Losing an
`app_state_changed` event delays drain discovery to the next poll interval
(30 seconds default) but does not affect safety or eventual completion.

### Validator and EVM Reader Exclusion

The spec's `ServiceNames` is `{"advancer", "claimer", "prt"}`. The Validator
and EVM Reader are excluded because they hold no cross-tick state that requires
drain coordination. The Validator computes claims purely from DB state (no
persistent machine or in-flight L1 tx). The EVM Reader monitors blockchain
events and writes to the DB but holds no per-application volatile state that
could conflict with deletion. Both services naturally stop processing an app
when it leaves the active set.

## Background

This specification was produced through a five-agent adversarial analysis
(see `drain-protocol-spec.md`). That document is a historical analysis log.
This spec models the corrected protocol after the design revisions and
implementation fixes identified there.

See `drain-protocol-spec.md` Section 7 for the TLC iteration log and
Section 2.6 for the design question resolutions.

## Mapping to the Go Codebase

| Formal Concept | Go Package | Key Types/Functions |
|----------------|-----------|---------------------|
| Application lifecycle | `internal/repository/postgres/lifecycle.go` | `SetApplicationEnabled`, `SoftDeleteApplication`, `HardDeleteApplication` |
| Drain acks | `internal/repository/postgres/lifecycle.go` | `AcknowledgeAppStopped`, `GetPendingAcks` |
| Required services | `internal/repository/repository.go` | `DrainServicesForConsensus`, `ConsensusTypesForService` |
| Advancer drain | `internal/advancer/advancer.go` | `Step()` scans soft-deleted apps and acks after machine teardown |
| Claimer drain | `internal/claimer/service.go` | `Tick()` scans soft-deleted apps and defers ack while claims are in flight |
| PRT drain | `internal/prt/service.go` | `Tick()` scans soft-deleted apps and defers ack while settle/join txs are in flight |
| Tournament check | `internal/repository/postgres/tournament.go` | `HasActiveTournaments` |
| Remove CLI | `cmd/cartesi-rollups-cli/root/app/remove/remove.go` | soft-delete flow, bond-loss guard, consensus-aware ack warnings |
| Purge CLI | `cmd/cartesi-rollups-cli/root/app/purge/purge.go` | consensus-aware ack check before hard delete |
