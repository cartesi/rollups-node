------------------------ MODULE TraceHybridEvents ------------------------
(*
  Trace validation spec for HybridEvents.

  This module validates that a recorded execution trace (from the Go
  implementation) conforms to the HybridEvents specification. It works by:

    1. Constraining the initial state to match the first trace entry.
    2. At each step, constraining the post-state to match the next trace entry.
    3. Checking that some action from HybridEvents.Next can produce the
       observed state transition.

  If TLC finds a deadlock before reaching the end of the trace, it means
  some transition in the trace cannot be explained by any TLA+ action —
  the implementation deviated from the spec.

  Run with:
    java -jar tla2tools.jar -config TraceHybridEvents.cfg \
         -deadlock -workers auto TraceHybridEvents

  The -deadlock flag suppresses the expected deadlock at the end of the
  trace (the trace is finite).
*)

EXTENDS HybridEvents, Trace

VARIABLE step

tracevars == <<vars, step>>

---------------------------------------------------------------------------
(* Map a trace entry's fields to TLA+ variables. *)

MapVariables(i) ==
    /\ db = trace[i].db
    /\ eventChannel = trace[i].eventChannel
    /\ workerState = trace[i].workerState
    /\ workerClock = trace[i].workerClock
    /\ processed = trace[i].processed
    /\ producerNext = trace[i].producerNext

MapVariablesPrimed(i) ==
    /\ db' = trace[i].db
    /\ eventChannel' = trace[i].eventChannel
    /\ workerState' = trace[i].workerState
    /\ workerClock' = trace[i].workerClock
    /\ processed' = trace[i].processed
    /\ producerNext' = trace[i].producerNext

---------------------------------------------------------------------------
(* Trace-constrained specification *)

TraceInit ==
    /\ step = 1
    /\ MapVariables(1)

TraceNext ==
    /\ step < Len(trace)
    /\ step' = step + 1
    \* The post-state must match the next trace entry.
    /\ MapVariablesPrimed(step + 1)
    \* AND some action from the original spec must be consistent with
    \* this transition (same pre-state, action constraints hold).
    /\ \/ ProduceItem
       \/ \E w \in Workers : EventWakeup(w)
       \/ \E w \in Workers : SyncWakeup(w)
       \/ ClockTick
       \/ \E w \in Workers : WorkerCrash(w)
       \/ \E w \in Workers : WorkerRestart(w)

TraceSpec == TraceInit /\ [][TraceNext]_tracevars

===========================================================================
