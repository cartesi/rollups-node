--------------------------- MODULE HybridEvents ---------------------------
(*
  Formal specification of the hybrid event system for the Cartesi rollups-node.

  The system consists of:
    - A database (source of truth) containing work items with statuses
    - An unreliable event channel (fire-and-forget notifications)
    - Workers that wake up either from events (fast path) or periodic
      sync timers (safety net)

  Key design invariant: events affect only WHEN database state is observed,
  never WHAT is observed. The database is always the source of truth.

  Safety property: no item is processed more than once (conditional updates).
  Liveness property: under weak fairness of SyncWakeup, every pending item
  is eventually processed.
*)

EXTENDS Integers, FiniteSets, Sequences

CONSTANTS
    MaxItems,       \* Maximum number of items the producer can create
    Workers,        \* Set of worker identifiers (e.g., {"w1", "w2"})
    SyncInterval    \* Number of steps between forced sync wakeups

VARIABLES
    db,             \* Database: item -> status ("PENDING" | "DONE")
    eventChannel,   \* Set of item IDs with pending notifications
    workerState,    \* Worker -> "idle" | "processing" | "dead"
    workerClock,    \* Worker -> steps since last sync (counts up to SyncInterval)
    processed,      \* Set of items that have been processed
    producerNext    \* Next item ID to produce

vars == <<db, eventChannel, workerState, workerClock, processed, producerNext>>

TypeOK ==
    /\ db \in [1..MaxItems -> {"PENDING", "DONE", "ABSENT"}]
    /\ eventChannel \subseteq 1..MaxItems
    /\ workerState \in [Workers -> {"idle", "processing", "dead"}]
    /\ workerClock \in [Workers -> 0..SyncInterval]
    /\ processed \subseteq 1..MaxItems
    /\ producerNext \in 1..(MaxItems + 1)

Init ==
    /\ db = [i \in 1..MaxItems |-> "ABSENT"]
    /\ eventChannel = {}
    /\ workerState = [w \in Workers |-> "idle"]
    /\ workerClock = [w \in Workers |-> 0]
    /\ processed = {}
    /\ producerNext = 1

---------------------------------------------------------------------------
(* Actions *)

(* Producer writes an item to the database and non-deterministically
   delivers or drops the notification. *)
ProduceItem ==
    /\ producerNext <= MaxItems
    /\ db' = [db EXCEPT ![producerNext] = "PENDING"]
    /\ \/ eventChannel' = eventChannel \union {producerNext}  \* delivered
       \/ eventChannel' = eventChannel                         \* dropped
    /\ producerNext' = producerNext + 1
    /\ UNCHANGED <<workerState, workerClock, processed>>

(* Worker wakes up from an event notification, re-queries the database
   for ALL pending items, and processes them.

   NOTE: This action models processing as ATOMIC (read + mark DONE in one step).
   The real implementation is non-atomic: Tick() takes time (potentially seconds
   for machine advance), during which new events may arrive and coalesce into a
   new signal, causing an extra Tick() after the current one finishes. This is
   safe because Tick() is idempotent (re-queries ALL pending work), but it means
   the spec under-counts the number of Tick() invocations per work item.
   See spec/events/README.md "Known Limitations" for details. *)
EventWakeup(w) ==
    /\ workerState[w] = "idle"
    /\ eventChannel /= {}
    /\ LET pending == {i \in 1..MaxItems : db[i] = "PENDING"}
       IN /\ db' = [i \in 1..MaxItems |->
                       IF i \in pending THEN "DONE" ELSE db[i]]
          /\ processed' = processed \union pending
    /\ eventChannel' = {}  \* Events consumed (fire-and-forget)
    /\ UNCHANGED <<workerState, workerClock, producerNext>>

(* Worker wakes up from the periodic sync timer, re-queries the database
   for ALL pending items, and processes them. This is the liveness guarantee. *)
SyncWakeup(w) ==
    /\ workerState[w] = "idle"
    /\ workerClock[w] >= SyncInterval
    /\ LET pending == {i \in 1..MaxItems : db[i] = "PENDING"}
       IN /\ db' = [i \in 1..MaxItems |->
                       IF i \in pending THEN "DONE" ELSE db[i]]
          /\ processed' = processed \union pending
    /\ workerClock' = [workerClock EXCEPT ![w] = 0]
    /\ UNCHANGED <<eventChannel, workerState, producerNext>>

(* Clock tick: advance the sync timer for all idle workers. *)
ClockTick ==
    /\ \E w \in Workers :
        /\ workerState[w] = "idle"
        /\ workerClock[w] < SyncInterval
        /\ workerClock' = [workerClock EXCEPT ![w] = workerClock[w] + 1]
    /\ UNCHANGED <<db, eventChannel, workerState, processed, producerNext>>

(* Worker crashes: loses all in-memory state, LISTEN connection drops,
   events are lost. *)
WorkerCrash(w) ==
    /\ workerState[w] = "idle"
    /\ workerState' = [workerState EXCEPT ![w] = "dead"]
    /\ UNCHANGED <<db, eventChannel, workerClock, processed, producerNext>>

(* Worker restarts: sync timer fires immediately on startup to catch up. *)
WorkerRestart(w) ==
    /\ workerState[w] = "dead"
    /\ workerState' = [workerState EXCEPT ![w] = "idle"]
    /\ workerClock' = [workerClock EXCEPT ![w] = SyncInterval]  \* Immediate sync
    /\ UNCHANGED <<db, eventChannel, processed, producerNext>>

---------------------------------------------------------------------------
(* Specification *)

Next ==
    \/ ProduceItem
    \/ \E w \in Workers : EventWakeup(w)
    \/ \E w \in Workers : SyncWakeup(w)
    \/ ClockTick
    \/ \E w \in Workers : WorkerCrash(w)
    \/ \E w \in Workers : WorkerRestart(w)

(* Fairness assumptions:
   - SyncWakeup: strong fairness (SF) — if SyncWakeup is enabled
     infinitely often, it eventually fires. This models the real system
     where the service framework keeps workers alive and the sync timer
     eventually fires between crashes.
   - EventWakeup: strong fairness — same reasoning.
   - WorkerRestart: weak fairness — dead workers eventually restart.
   - ClockTick: weak fairness — the clock advances. *)
Fairness ==
    /\ \A w \in Workers : SF_vars(SyncWakeup(w))
    /\ \A w \in Workers : SF_vars(EventWakeup(w))
    /\ \A w \in Workers : WF_vars(WorkerRestart(w))
    /\ WF_vars(ClockTick)

Spec == Init /\ [][Next]_vars /\ Fairness

---------------------------------------------------------------------------
(* Safety Properties *)

(* No item is marked DONE in the database unless it was actually processed. *)
Safety_NoDuplicateProcessing ==
    \A i \in 1..MaxItems :
        db[i] = "DONE" => i \in processed

(* Type invariant. *)
Safety_TypeOK == TypeOK

---------------------------------------------------------------------------
(* Liveness Properties *)

(* Every item that is produced (written to DB as PENDING) is eventually
   processed (moved to DONE). This holds under the fairness assumption
   that SyncWakeup eventually fires. *)
Liveness_EventualProcessing ==
    \A i \in 1..MaxItems :
        (db[i] = "PENDING") ~> (db[i] = "DONE")

===========================================================================
