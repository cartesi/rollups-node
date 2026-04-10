------------------------ MODULE DrainProtocol ------------------------
(*
  Formal specification of the drain protocol for the Cartesi rollups-node.

  The drain protocol coordinates safe removal of an application across
  multiple services that hold cross-tick state (machine processes,
  in-flight L1 transactions). Each service must acknowledge ("ack") that
  it has finished all in-flight work before the application's data can be
  permanently deleted.

  The protocol's state is split across two storage domains:
    - PostgreSQL (persistent): enabled, deleted_at, ack rows
    - Per-service memory (volatile): machines, in-flight L1 transactions

  The key design invariant: the database is the single source of truth
  for application lifecycle. On each tick, services scan the database for
  all soft-deleted apps they are required for and ack them.

  Application lifecycle:
    - Enable/disable is a reversible operational toggle. Disable stops
      processing and frees resources (machines destroyed) but does NOT
      trigger drain. Disabled apps can be re-enabled (machines rebuilt
      from snapshot). Disabled apps may remain as archives indefinitely.
    - Drain starts when the operator soft-deletes (sets deleted_at).
      This is the irreversible "point of no return." Services detect
      soft-delete, perform cleanup, and write acks.
    - Hard-delete (purge) requires all acks, or explicit force-purge.

  Design decisions encoded in this spec:
    D2: Scan-on-every-tick is the ack mechanism (no diff, no startup scan).
    D3: Health state (STOPPED/FAILED) is not modeled (irrelevant to drain).
    D4: PRT apps with active tournaments cannot be disabled/removed
        without explicit operator override (SafeToDisable guard).

  See drain-protocol-spec.md for the full analysis.
*)

EXTENDS Integers, FiniteSets

CONSTANTS
    AppIDs,                \* Set of application IDs, e.g. {1, 2}
    ServiceNames,          \* {"advancer", "claimer", "prt"}
    ConsensusType,         \* Function: AppIDs -> {"AUTHORITY", "PRT"}
    PrtTournamentActive    \* Function: AppIDs -> BOOLEAN (on-chain state abstraction)
                           \* Modeled as a constant because the PRT service does not
                           \* join new tournaments for disabled apps (PRTVisible filter
                           \* returns false when enabled=false). This assumption is
                           \* enforced by the PRTSubmitTx precondition, which requires
                           \* PRTVisible(a).

VARIABLES
    \* --- Database state (persistent across crashes) ---

    dbEnabled,             \* [AppIDs -> BOOLEAN]
    dbDeleted,             \* [AppIDs -> BOOLEAN]  (abstracts deleted_at)
    dbAcks,                \* [AppIDs -> SUBSET ServiceNames]
    dbHardDeleted,         \* SUBSET AppIDs  (row removed from DB)
    forceDeleted,          \* SUBSET AppIDs  (apps hard-deleted via force purge)

    \* --- Advancer volatile state ---

    advMachines,           \* SUBSET AppIDs  (apps with live machine processes)

    \* --- Claimer volatile state ---

    clmKnown,              \* SUBSET AppIDs  (active Authority/Quorum apps tracked)
    clmInFlight,           \* SUBSET AppIDs  (apps with pending L1 claim tx)

    \* --- PRT volatile state ---

    prtKnown,              \* SUBSET AppIDs  (active PRT apps tracked)
    prtInFlight,           \* SUBSET AppIDs  (apps with pending L1 settle/join tx)

    \* --- Service liveness ---

    svcAlive               \* [ServiceNames -> BOOLEAN]

vars == <<dbEnabled, dbDeleted, dbAcks, dbHardDeleted, forceDeleted,
          advMachines, clmKnown, clmInFlight, prtKnown, prtInFlight,
          svcAlive>>

---------------------------------------------------------------------------
(* Helper Predicates *)

\* Apps that have not been hard-deleted.
Alive == AppIDs \ dbHardDeleted

\* What the DB "Active" filter returns:
\*   enabled = true AND deleted_at IS NULL
\* (Health is not modeled -- see D3.)
IsActive(a) ==
    /\ a \notin dbHardDeleted
    /\ dbEnabled[a]
    /\ ~dbDeleted[a]

\* Consensus-aware required services.
\*   Authority/Quorum -> {"advancer", "claimer"}
\*   PRT              -> {"advancer", "prt"}
RequiredServices(a) ==
    IF ConsensusType[a] = "PRT"
    THEN {"advancer", "prt"}
    ELSE {"advancer", "claimer"}

\* Are all required acks present for app a?
AllAcked(a) == RequiredServices(a) \subseteq dbAcks[a]

\* What the claimer's DB query returns (filters out PRT apps).
ClaimerVisible(a) ==
    /\ IsActive(a)
    /\ ConsensusType[a] # "PRT"

\* What the PRT service's DB query returns (filters out Authority apps).
PRTVisible(a) ==
    /\ IsActive(a)
    /\ ConsensusType[a] = "PRT"

\* Is it safe to disable this app? (D4)
\* Authority/Quorum: always safe.
\* PRT: safe only when no active tournaments exist (no bonded ETH at risk).
SafeToDisable(a) ==
    \/ ConsensusType[a] # "PRT"
    \/ ~PrtTournamentActive[a]

---------------------------------------------------------------------------
(* Initial State *)

Init ==
    /\ dbEnabled     = [a \in AppIDs |-> TRUE]
    /\ dbDeleted     = [a \in AppIDs |-> FALSE]
    /\ dbAcks        = [a \in AppIDs |-> {}]
    /\ dbHardDeleted = {}
    /\ forceDeleted  = {}
    /\ advMachines   = AppIDs    \* All apps start with machines
    /\ clmKnown      = {}        \* Empty on first startup
    /\ clmInFlight   = {}
    /\ prtKnown      = {}
    /\ prtInFlight   = {}
    /\ svcAlive      = [s \in ServiceNames |-> TRUE]

---------------------------------------------------------------------------
(* Operator Actions *)

(* Operator disables an application (reversible operational toggle).
   Stops processing, frees resources (machines destroyed on next tick).
   Does NOT trigger drain — disabled apps can be re-enabled.
   Blocked for PRT apps with active tournaments unless force-override.
   Maps to: SetApplicationEnabled(ctx, appID, false) in lifecycle.go *)
OperatorDisable(a) ==
    /\ a \in Alive
    /\ dbEnabled[a] = TRUE
    /\ ~dbDeleted[a]
    /\ SafeToDisable(a)
    /\ dbEnabled' = [dbEnabled EXCEPT ![a] = FALSE]
    /\ UNCHANGED <<dbDeleted, dbAcks, dbHardDeleted, forceDeleted,
                    advMachines, clmKnown, clmInFlight,
                    prtKnown, prtInFlight, svcAlive>>

(* Operator force-disables a PRT app with active tournaments.
   Maps to: app disable --acknowledge-bond-loss *)
OperatorForceDisable(a) ==
    /\ a \in Alive
    /\ dbEnabled[a] = TRUE
    /\ ~dbDeleted[a]
    /\ ~SafeToDisable(a)
    /\ dbEnabled' = [dbEnabled EXCEPT ![a] = FALSE]
    /\ UNCHANGED <<dbDeleted, dbAcks, dbHardDeleted, forceDeleted,
                    advMachines, clmKnown, clmInFlight,
                    prtKnown, prtInFlight, svcAlive>>

(* Operator re-enables a disabled application.
   Blocked once soft-deleted (point of no return for drain).
   Health guards (e.g., INOPERABLE blocks re-enable) are not modeled (D3).
   Maps to: SetApplicationEnabled(ctx, appID, true) in lifecycle.go *)
OperatorReEnable(a) ==
    /\ a \in Alive
    /\ ~dbEnabled[a]
    /\ ~dbDeleted[a]
    /\ dbEnabled' = [dbEnabled EXCEPT ![a] = TRUE]
    /\ UNCHANGED <<dbDeleted, dbAcks, dbHardDeleted, forceDeleted,
                    advMachines, clmKnown, clmInFlight,
                    prtKnown, prtInFlight, svcAlive>>

(* Operator soft-deletes an application, initiating drain.
   This is the irreversible "point of no return." Atomically sets
   enabled=false (if not already) and deleted_at. Services detect
   deleted_at on their next tick and begin cleanup + ack.
   Works on both enabled and disabled apps. For enabled PRT apps
   with active tournaments, SafeToDisable guard applies.
   Maps to: app remove (or app remove --force for enabled apps) *)
OperatorSoftDelete(a) ==
    /\ a \in Alive
    /\ ~dbDeleted[a]
    /\ (dbEnabled[a] => SafeToDisable(a))
    /\ dbEnabled' = [dbEnabled EXCEPT ![a] = FALSE]
    /\ dbDeleted' = [dbDeleted EXCEPT ![a] = TRUE]
    /\ UNCHANGED <<dbAcks, dbHardDeleted, forceDeleted,
                    advMachines, clmKnown, clmInFlight,
                    prtKnown, prtInFlight, svcAlive>>

(* Operator force-removes an enabled PRT app with active tournaments.
   Atomically disables and soft-deletes, bypassing SafeToDisable.
   Maps to: app remove --acknowledge-bond-loss *)
OperatorForceSoftDelete(a) ==
    /\ a \in Alive
    /\ ~dbDeleted[a]
    /\ dbEnabled[a]
    /\ ~SafeToDisable(a)
    /\ dbEnabled' = [dbEnabled EXCEPT ![a] = FALSE]
    /\ dbDeleted' = [dbDeleted EXCEPT ![a] = TRUE]
    /\ UNCHANGED <<dbAcks, dbHardDeleted, forceDeleted,
                    advMachines, clmKnown, clmInFlight,
                    prtKnown, prtInFlight, svcAlive>>

(* Operator hard-deletes after verifying all acks (purge).
   Maps to: HardDeleteApplication(ctx, appID) in purge.go *)
OperatorPurge(a) ==
    /\ a \in Alive
    /\ dbDeleted[a] = TRUE
    /\ AllAcked(a)
    /\ dbHardDeleted' = dbHardDeleted \union {a}
    /\ UNCHANGED <<dbEnabled, dbDeleted, dbAcks, forceDeleted,
                    advMachines, clmKnown, clmInFlight,
                    prtKnown, prtInFlight, svcAlive>>

(* Operator force-purges: hard-delete bypassing ack check.
   Maps to: purge --force CLI command.
   The app is tracked in forceDeleted so Safety_PurgeRequiresAcks
   can distinguish force-purge from a bug that deletes without acks.
   WARNING: If in-flight L1 transactions exist (clmInFlight or prtInFlight),
   force-purge orphans them. For PRT apps with bonded tournaments, this can
   cause bond forfeiture — the node can no longer participate in the tournament
   because machine snapshots and epoch data are CASCADE-deleted with the app. *)
OperatorForcePurge(a) ==
    /\ a \in Alive
    /\ dbDeleted[a] = TRUE
    /\ dbHardDeleted' = dbHardDeleted \union {a}
    /\ forceDeleted' = forceDeleted \union {a}
    /\ UNCHANGED <<dbEnabled, dbDeleted, dbAcks,
                    advMachines, clmKnown, clmInFlight,
                    prtKnown, prtInFlight, svcAlive>>

---------------------------------------------------------------------------
(* Service Tick Actions *)

(* Advancer tick: updates machines to match active apps and acks all
   soft-deleted apps it is required for.
   Machines exist only for active apps — destroyed on disable AND on
   soft-delete. Acks are written only for soft-deleted apps (drain).
   Maps to: Step() in advancer.go *)
AdvancerTick ==
    /\ svcAlive["advancer"]
    /\ LET active == {a \in Alive : IsActive(a)}
           toAck == {a \in Alive :
                /\ dbDeleted[a]
                /\ "advancer" \in RequiredServices(a)}
       IN
       /\ advMachines' = active
       /\ dbAcks' = [a \in AppIDs |->
            IF a \in toAck
            THEN dbAcks[a] \union {"advancer"}
            ELSE dbAcks[a]]
       /\ UNCHANGED <<dbEnabled, dbDeleted, dbHardDeleted, forceDeleted,
                       clmKnown, clmInFlight, prtKnown, prtInFlight,
                       svcAlive>>

(* Claimer tick: acks soft-deleted Authority/Quorum apps,
   deferring if an in-flight L1 claim exists.
   Maps to: Tick() in claimer/service.go *)
ClaimerTick ==
    /\ svcAlive["claimer"]
    /\ LET visible == {a \in Alive : ClaimerVisible(a)}
           toAck == {a \in Alive :
                /\ dbDeleted[a]
                /\ "claimer" \in RequiredServices(a)} \ clmInFlight
       IN
       /\ dbAcks' = [a \in AppIDs |->
            IF a \in toAck
            THEN dbAcks[a] \union {"claimer"}
            ELSE dbAcks[a]]
       /\ clmKnown' = visible
       /\ clmInFlight' = clmInFlight \intersect visible
       /\ UNCHANGED <<dbEnabled, dbDeleted, dbHardDeleted, forceDeleted,
                       advMachines, prtKnown, prtInFlight, svcAlive>>

(* PRT tick: acks soft-deleted PRT apps,
   deferring if an in-flight L1 tx exists.
   Maps to: Tick() in prt/service.go *)
PRTTick ==
    /\ svcAlive["prt"]
    /\ LET visible == {a \in Alive : PRTVisible(a)}
           toAck == {a \in Alive :
                /\ dbDeleted[a]
                /\ "prt" \in RequiredServices(a)} \ prtInFlight
       IN
       /\ dbAcks' = [a \in AppIDs |->
            IF a \in toAck
            THEN dbAcks[a] \union {"prt"}
            ELSE dbAcks[a]]
       /\ prtKnown' = visible
       /\ prtInFlight' = prtInFlight \intersect visible
       /\ UNCHANGED <<dbEnabled, dbDeleted, dbHardDeleted, forceDeleted,
                       advMachines, clmKnown, clmInFlight, svcAlive>>

---------------------------------------------------------------------------
(* L1 Transaction Actions *)

ClaimerSubmitClaim(a) ==
    /\ svcAlive["claimer"]
    /\ a \in clmKnown
    /\ a \notin clmInFlight
    /\ ClaimerVisible(a)
    /\ clmInFlight' = clmInFlight \union {a}
    /\ UNCHANGED <<dbEnabled, dbDeleted, dbAcks, dbHardDeleted, forceDeleted,
                    advMachines, clmKnown, prtKnown, prtInFlight, svcAlive>>

ClaimerClaimConfirmed(a) ==
    /\ a \in clmInFlight
    /\ clmInFlight' = clmInFlight \ {a}
    /\ UNCHANGED <<dbEnabled, dbDeleted, dbAcks, dbHardDeleted, forceDeleted,
                    advMachines, clmKnown, prtKnown, prtInFlight, svcAlive>>

PRTSubmitTx(a) ==
    /\ svcAlive["prt"]
    /\ a \in prtKnown
    /\ PRTVisible(a)
    /\ prtInFlight' = prtInFlight \union {a}
    /\ UNCHANGED <<dbEnabled, dbDeleted, dbAcks, dbHardDeleted, forceDeleted,
                    advMachines, clmKnown, clmInFlight, prtKnown, svcAlive>>

PRTTxConfirmed(a) ==
    /\ a \in prtInFlight
    /\ prtInFlight' = prtInFlight \ {a}
    /\ UNCHANGED <<dbEnabled, dbDeleted, dbAcks, dbHardDeleted, forceDeleted,
                    advMachines, clmKnown, clmInFlight, prtKnown, svcAlive>>

---------------------------------------------------------------------------
(* Crash / Restart Actions *)

ServiceCrash(s) ==
    /\ svcAlive[s]
    /\ svcAlive' = [svcAlive EXCEPT ![s] = FALSE]
    /\ IF s = "advancer"
       THEN /\ advMachines' = {}
            /\ UNCHANGED <<clmKnown, clmInFlight, prtKnown, prtInFlight>>
       ELSE IF s = "claimer"
       THEN /\ clmKnown' = {}
            /\ clmInFlight' = {}
            /\ UNCHANGED <<advMachines, prtKnown, prtInFlight>>
       ELSE \* prt
            /\ prtKnown' = {}
            /\ prtInFlight' = {}
            /\ UNCHANGED <<advMachines, clmKnown, clmInFlight>>
    /\ UNCHANGED <<dbEnabled, dbDeleted, dbAcks, dbHardDeleted, forceDeleted>>

ServiceRestart(s) ==
    /\ ~svcAlive[s]
    /\ svcAlive' = [svcAlive EXCEPT ![s] = TRUE]
    /\ UNCHANGED <<dbEnabled, dbDeleted, dbAcks, dbHardDeleted, forceDeleted,
                    advMachines, clmKnown, clmInFlight,
                    prtKnown, prtInFlight>>

---------------------------------------------------------------------------
(* Next-State Relation *)

Next ==
    \/ \E a \in Alive :
        \/ OperatorDisable(a)
        \/ OperatorForceDisable(a)
        \/ OperatorReEnable(a)
        \/ OperatorSoftDelete(a)
        \/ OperatorForceSoftDelete(a)
        \/ OperatorPurge(a)
        \/ OperatorForcePurge(a)
        \/ ClaimerSubmitClaim(a)
        \/ ClaimerClaimConfirmed(a)
        \/ PRTSubmitTx(a)
        \/ PRTTxConfirmed(a)
    \/ AdvancerTick
    \/ ClaimerTick
    \/ PRTTick
    \/ \E s \in ServiceNames : ServiceCrash(s)
    \/ \E s \in ServiceNames : ServiceRestart(s)

---------------------------------------------------------------------------
(* Safety Properties *)

(* Hard delete only occurs when all required acks are present,
   unless the operator used force-purge (bypassing the ack check). *)
Safety_PurgeRequiresAcks ==
    \A a \in dbHardDeleted : AllAcked(a) \/ a \in forceDeleted

(* If the advancer has acked, it holds no machine for the app. *)
Safety_AdvAckImpliesNoMachine ==
    \A a \in Alive :
        "advancer" \in dbAcks[a] => a \notin advMachines

(* If the claimer has acked, it has no in-flight claim for the app. *)
Safety_ClmAckImpliesNoInFlight ==
    \A a \in Alive :
        "claimer" \in dbAcks[a] => a \notin clmInFlight

(* If PRT has acked, it has no in-flight L1 tx for the app. *)
Safety_PRTAckImpliesNoInFlight ==
    \A a \in Alive :
        "prt" \in dbAcks[a] => a \notin prtInFlight

(* Non-deleted apps have no ack rows. Drain acks are only written for
   soft-deleted apps. Since re-enable requires ~dbDeleted, stale acks
   from a previous drain cycle are structurally impossible — there is
   no drain cycle for non-deleted apps. *)
Safety_NoAcksBeforeDrain ==
    \A a \in Alive :
        ~dbDeleted[a] => dbAcks[a] = {}

(* Force-deleted apps are a subset of hard-deleted apps.
   Defensive invariant: catches bugs where forceDeleted is populated
   without a corresponding hard delete. *)
Safety_ForceImpliesHardDeleted ==
    forceDeleted \subseteq dbHardDeleted

(* Soft-deleted apps are necessarily disabled. Holds by construction
   (OperatorSoftDelete atomically sets enabled=false) but stated as
   a regression check. *)
Safety_DeletedImpliesDisabled ==
    \A a \in Alive :
        dbDeleted[a] => ~dbEnabled[a]

---------------------------------------------------------------------------
(* Liveness Properties *)

(* Every soft-deleted app is eventually fully acked by all required
   services, or explicitly force-purged by the operator.
   Soft-delete is irreversible (no un-delete), so this is a true
   one-way obligation. Disabled-but-not-deleted apps have no liveness
   obligation — they can remain as archives indefinitely. *)
Liveness_DrainCompletes ==
    \A a \in AppIDs :
        (dbDeleted[a] /\ a \notin dbHardDeleted)
        ~> (AllAcked(a) \/ a \in forceDeleted)

(* After force-purge, services eventually clean up orphaned volatile state
   (machines, in-flight L1 transactions) because their tick actions recompute
   active sets from the database, which no longer contains the purged app. *)
Liveness_ForcePurgeCleanup ==
    \A a \in AppIDs :
        a \in forceDeleted
        ~> (a \notin advMachines /\ a \notin clmInFlight /\ a \notin prtInFlight)

---------------------------------------------------------------------------
(* Fairness *)

\* Service ticks use strong fairness (SF) because a crash temporarily
\* disables the tick. SF guarantees: if enabled infinitely often, it
\* eventually fires. Same reasoning as HybridEvents spec.
\*
\* Service restarts and L1 confirmations use weak fairness (WF).
Fairness ==
    /\ SF_vars(AdvancerTick)
    /\ SF_vars(ClaimerTick)
    /\ SF_vars(PRTTick)
    /\ \A s \in ServiceNames : WF_vars(ServiceRestart(s))
    /\ \A a \in AppIDs : WF_vars(ClaimerClaimConfirmed(a))
    /\ \A a \in AppIDs : WF_vars(PRTTxConfirmed(a))

Spec == Init /\ [][Next]_vars /\ Fairness

===========================================================================
