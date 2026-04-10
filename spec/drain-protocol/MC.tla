------------------------------ MODULE MC ------------------------------
(*
  Model-checking wrapper for DrainProtocol.

  Two apps: one Authority/Quorum (#1) and one PRT (#2).
  App 2 has an active tournament, testing that normal disable is blocked
  and OperatorForceDisable is required.

  Fairness conditions expanded for concrete sets (TLC requirement).
*)

EXTENDS DrainProtocol, TLC

MC_AppIDs == {1, 2}
MC_ServiceNames == {"advancer", "claimer", "prt"}
MC_ConsensusType == (1 :> "AUTHORITY" @@ 2 :> "PRT")
MC_PrtTournamentActive == (1 :> FALSE @@ 2 :> TRUE)

MC_Fairness ==
    /\ SF_vars(AdvancerTick)
    /\ SF_vars(ClaimerTick)
    /\ SF_vars(PRTTick)
    /\ WF_vars(ServiceRestart("advancer"))
    /\ WF_vars(ServiceRestart("claimer"))
    /\ WF_vars(ServiceRestart("prt"))
    /\ WF_vars(ClaimerClaimConfirmed(1))
    /\ WF_vars(ClaimerClaimConfirmed(2))
    /\ WF_vars(PRTTxConfirmed(1))
    /\ WF_vars(PRTTxConfirmed(2))

MC_Spec == Init /\ [][Next]_vars /\ MC_Fairness

===========================================================================
