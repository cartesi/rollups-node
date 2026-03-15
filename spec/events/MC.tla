-------------------------------- MODULE MC --------------------------------
(*
  Model-checking wrapper for HybridEvents.

  Provides concrete constant values for TLC model checking.
  State space is approximately 10^5 states with these parameters.

  The fairness conditions are expanded explicitly here because TLC
  cannot enumerate CONSTANT sets in temporal quantifiers.
*)

EXTENDS HybridEvents

MC_MaxItems == 3
MC_Workers == {"w1", "w2"}
MC_SyncInterval == 2

(* Expand fairness for the concrete worker set so TLC can check
   temporal properties. This is equivalent to the Fairness definition
   in HybridEvents but with Workers expanded.
   SF = strong fairness: if enabled infinitely often, eventually fires.
   WF = weak fairness: if continuously enabled, eventually fires. *)
MC_Fairness ==
    /\ SF_vars(SyncWakeup("w1"))
    /\ SF_vars(SyncWakeup("w2"))
    /\ SF_vars(EventWakeup("w1"))
    /\ SF_vars(EventWakeup("w2"))
    /\ WF_vars(WorkerRestart("w1"))
    /\ WF_vars(WorkerRestart("w2"))
    /\ WF_vars(ClockTick)

MC_Spec == Init /\ [][Next]_vars /\ MC_Fairness

===========================================================================
