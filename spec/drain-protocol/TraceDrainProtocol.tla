------------------------ MODULE TraceDrainProtocol ------------------------
(*
  Trace validation spec for DrainProtocol.

  This module validates that a recorded execution trace (from the Go
  in-memory simulation) conforms to the DrainProtocol specification.
  It constrains each state to match the generated trace while requiring
  that every observed transition is explained by some DrainProtocol action.

  Run with:
    java -jar tla2tools.jar -config TraceDrainProtocol.cfg \
         -deadlock -workers auto TraceDrainProtocol

  The -deadlock flag suppresses the expected deadlock at the end of the
  finite recorded trace.
*)

EXTENDS DrainProtocol, Trace, Sequences

VARIABLE step

Trace_AppIDs == {1, 2, 3}
Trace_ServiceNames == {"advancer", "claimer", "prt"}
Trace_ConsensusType == (1 :> "AUTHORITY" @@ 2 :> "QUORUM" @@ 3 :> "PRT")
Trace_PrtTournamentActive == (1 :> FALSE @@ 2 :> FALSE @@ 3 :> TRUE)

tracevars == <<vars, step>>

----------------------------------------------------------------------------
(* Map a trace entry's fields to TLA+ variables. *)

MapVariables(i) ==
    /\ dbEnabled = trace[i].dbEnabled
    /\ dbDeleted = trace[i].dbDeleted
    /\ dbAcks = trace[i].dbAcks
    /\ dbHardDeleted = trace[i].dbHardDeleted
    /\ forceDeleted = trace[i].forceDeleted
    /\ advMachines = trace[i].advMachines
    /\ clmKnown = trace[i].clmKnown
    /\ clmInFlight = trace[i].clmInFlight
    /\ prtKnown = trace[i].prtKnown
    /\ prtInFlight = trace[i].prtInFlight
    /\ svcAlive = trace[i].svcAlive

MapVariablesPrimed(i) ==
    /\ dbEnabled' = trace[i].dbEnabled
    /\ dbDeleted' = trace[i].dbDeleted
    /\ dbAcks' = trace[i].dbAcks
    /\ dbHardDeleted' = trace[i].dbHardDeleted
    /\ forceDeleted' = trace[i].forceDeleted
    /\ advMachines' = trace[i].advMachines
    /\ clmKnown' = trace[i].clmKnown
    /\ clmInFlight' = trace[i].clmInFlight
    /\ prtKnown' = trace[i].prtKnown
    /\ prtInFlight' = trace[i].prtInFlight
    /\ svcAlive' = trace[i].svcAlive

----------------------------------------------------------------------------
(* Trace-constrained specification *)

TraceInit ==
    /\ step = 1
    /\ MapVariables(1)

TraceNext ==
    /\ step < Len(trace)
    /\ step' = step + 1
    /\ MapVariablesPrimed(step + 1)
    /\ \/ \E a \in Alive :
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

TraceSpec == TraceInit /\ [][TraceNext]_tracevars

============================================================================
