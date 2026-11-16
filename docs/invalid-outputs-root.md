# Invalid outputs root handling

The application supplies an outputs root in the machine state. The node also calculates a cumulative root from
its stored output bytes. These two values have separate roles:

| Epoch field | Meaning |
| --- | --- |
| `tx_buffer_data_block` | The root declared in the proved machine state. Claim observation compares this value and the machine hash with the chain result. |
| `calculated_outputs_root` | The cumulative root calculated from local outputs. The node stores it with local epoch completion and normal output proofs. |

`calculated_outputs_root = null` means that the local calculation is not available. It does not mean that the
declaration is valid or invalid. Present, unequal roots identify an invalid outputs root (IOR) relative to the
local output data. The root comparison alone does not identify the cause of the difference.

## Computation and actions

A final validator IOR does not stop true computation. For Dave, the node builds the PRT commitment from the
actual state hashes and stores it, the calculated root, the output proofs and `CLAIM_COMPUTED` in one transaction.
Authority and Quorum store the calculated root, output proofs and `CLAIM_COMPUTED`; the PRT commitment is specific
to Dave. A failure rolls back the result and its proof writes. The application health can remain `OK`.

| Operation | IOR epoch |
| --- | --- |
| Compute the true result and store normal proofs | Permitted when the required local sources are available. |
| Join the Dave root tournament with the true commitment | Permitted under the existing deployment, source, clock, funding and signer checks. |
| Stage or accept the Dave result | Refused, including accept after another party stages the result. |
| Authority submission/acceptance and Quorum vote/acceptance | Refused. |
| Observe another party's staging or acceptance | Continues. A matching machine hash and declared root do not cause divergence. |
| Query stored epochs, output bytes and proofs | Continues. A local output proof does not validate a different accepted declared root. |

Unknown roots also block publication and useful joins. A missing predecessor calculation makes an epoch with no
new outputs wait for adequate evidence. The node does not substitute the previous declared root.

An empty Dave epoch keeps the preceding machine state and declaration. If that declaration is wrong, the empty
epoch still has IOR. Its calculated root keeps the preceding calculated value. An epoch with inputs can declare
a corrected root; its own final comparison determines publication eligibility. There is no permanent publication
veto for every later epoch. Authority and Quorum can publish an eligible later claim without prior-claim acceptance.

Dave creates the next sealed epoch and tournament only when the current result is accepted. Inputs can already
accumulate and execute in the OPEN successor. If this node is the only settler and withholds an IOR result, the
next sealed tournament will not appear. Another party must stage and accept the winner for that progression.
Both calls are permissionless. A root winner makes its result available for settlement by an actor with the
machine validity proof; this node's veto does not stop another actor. A join alone does not establish a winner.
Without a participant and a winning candidate, the root can finish as `ROOT_FAILED` and cannot be staged.
The guardian can instead foreclose the application. A root winner does not expire. A join alone is not active
dispute defense; dispute moves are a separate component.

Bond recovery is separate from settlement. A finished root tournament can permit recovery without stage or
accept. The first join of a commitment fixes its claimer and recovery recipient. An actor that copies a join
and gets it included first pays its own bond and becomes that claimer. This does not transfer a bond already
paid by this node. The node recognizes the true commitment by its root; recovery still requires its signer to
be the recipient.

Real terminal inputs retain their execution-stop rules. A later terminal input does not replace an earlier
computed epoch's root evidence or remove that epoch's existing eligibility. Legacy manual accepted-root length
handling also remains separate from the final validator comparison.

## Operator signals

The Validator emits `condition=ior_settlement_withheld` after the result transaction commits. The record says
that local publication is prohibited; it does not say that the chain result is still unsettled. It includes
the application, epoch, both roots, application health, foreclosure block and epoch status.

After service restart, the Validator reads stored mismatches once for each enabled application that it first
encounters. It retries incomplete recovery without blocking otherwise eligible computation. Recovery includes
`FAILED`, `DIVERGED` and `CORRUPTED` applications; their computation and sending restrictions still apply.
Successful recovery is not a periodic historical scan. New findings are reported at their producer, and the
existing consensus observers report the current held winner, staging and acceptance. No reminder timer or alert history is stored.
These logs are diagnostics, not durable notifications: a crash or an uncertain commit response can suppress
the immediate message. The epoch fact and publication veto remain; restart recovery can report the stored fact.

Recovery uses the epoch row only. It reports `observation_source=stored` and `join_state=not_checked`,
with the commitment and tournament identities when available. These identities do not prove that a join
was sent or observed. Recovery does not read each commitment and tournament or report their standing.
Use the normal observers and API to inspect join evidence and the age of the tournament snapshot.
The shared detection point also reports `not_checked`.

When the PRT observer confirms that the current sealed epoch has a compatible tournament winner but its IOR
result is not staged, it emits `ior_winner_settlement_withheld` at ERROR. The node withholds local settlement;
without settlement by another actor, the next sealed epoch cannot start. Ask the guardian to review foreclosure.
The record identifies the epoch, tournament, winner and observation block/hash/policy. It does not call a join
active dispute defense. The same current winner is reported once per service instance, without a historical
inventory or a periodic reminder. Stage, acceptance or application inactivation retires the marker.

If join is not observed near or after the join-close block, check the coverage and age of the projection,
the transaction and the commitment on chain. A delayed projection can omit an included or copied join. Only
confirmed absence with joins closed rules out a new root join. It does not rule out defense of an existing join.

An observed staged IOR result has the distinct ERROR condition `ior_result_staged`. The record includes the
staging block, staging period and qualified observation evidence. A live Dave snapshot also reports
`sentries_agree` and `staging_period_over` at that observation block. These fields describe the snapshot; later
votes can change acceptance eligibility. That change does not emit another stage record; inspect current
chain state before acting. Recovery can also report a stored staged result; its source is marked
as stored and it does not invent the live sentry or period fields. The `foreclose_block` distinguishes a
historical stage after foreclosure from one that can still be accepted. This alert does not change application
health or permit local accept.
An observed matching acceptance retains IOR evidence and is reported at ERROR as `ior_result_accepted` by the
existing consensus observer, even when stage and acceptance are first seen together. Authority/Quorum signals
here refer to stored local epochs. Their independent observer can also compare claims for omitted empty windows;
these signals do not add IOR classification for such claims. Quorum's existing INFO for a matching foreign vote includes
`invalid_outputs_root` and the epoch index. A foreign vote alone does not mean the claim is staged.

For retained IOR epochs in `CLAIM_COMPUTED`, `CLAIM_SUBMITTED` and `CLAIM_STAGED`, the Authority/Quorum
observer reads the local claim first. It preserves normal event evidence, transaction hashes and comparisons
with other claim keys. A point read can confirm acceptance but cannot identify its transaction; only an
observed event supplies that transaction hash. With `finalized`, it keeps complete scan progress in memory.
Repeated visits at the same checked head do not rescan that history. Under `safe` and `latest`, each visit scans from the epoch
boundary again. Those modes keep their observation behavior but do not get this reduction of repeated RPC work.
Their work per visit grows with claims after the retained epoch; the scan routine imposes no fixed work limit.
Use `finalized` for deployments that can retain IOR epochs.

If a historical scan fails, a compatible positive point read can still supply a projection after its original
block reference is checked again. The scan error remains visible. A slow scan can delay this fallback until it
returns; cancellation or a hard observation failure prevents a new projection. An unavailable check of an old
finalized prefix preserves that entry for a retry, without using it or starting a full replay. A separately
verified positive point can still be reported. A non-canonical finalized prefix is a finality error.
That error stops claim publication and readiness until service restart. Observation can continue; a later
successful observation does not clear that publication stop.

Restart needs a complete historical scan for each retained IOR epoch. With K retained epochs, this work occurs
K times; it is not one shared catch-up scan. An Authority IOR epoch can stay `CLAIM_COMPUTED` while later healthy
claims settle. The observer processes its pages in sequence, so a long scan can delay other applications in
that observer. There is no partial cold-scan checkpoint or universal RPC budget. `safe` and `latest` repeat
this historical work on every visit, including repeated visits at the same head. The searches need contract
state at their historical query blocks. Missing historical state is a read failure, not proof of an empty range.

Failed or incomplete scans remain retryable; no persistent scan table is added. Provisional stage-event repair
reads its known staging block. A submitting Quorum node still checks its own vote history; it does not use a
peer's vote as its own. An orphaned audit hash can need a full repair scan. No provisional scan prefix is saved,
so a transient mixed-branch read cannot become a remembered promise that its history is complete. This does
not add rollback of stored provisional results or prove that every read in one visit came from the same branch.

Two shared helpers also apply to healthy epochs. Stage audit reads only the block supplied by the claim's
staging clock: the contract stores that block and emits the stage event there. Point reconciliation rejects
an invalid or future staging block and uses the applicable writer for `CLAIM_SUBMITTED` to `CLAIM_STAGED`.
These changes do not alter the outputs-root publication rule.

Live PRT result signals follow the existing health, deployment and prefix gates. If those gates stop
reconciliation, the node does not discover a new external stage through this signal. Startup recovery can
report an already stored staged result under stopped health; it does not scan the chain. A guest can cause
`FAILED` through malformed automatic Output/Report handling, which remains outside this delivery.

The Validator and the consensus observer can each report the same stored stage after restart. Alert routing
can group these records by application, epoch and staging block while keeping their observation sources.
Acceptance is not replayed under the `ior_result_accepted` name at restart: the Validator reports the stored
IOR fact with `epoch_status=CLAIM_ACCEPTED`. Recovery failures remain retryable and can warn again on each
visit until recovery succeeds.

A positive staging period is a minimum wait before acceptance when its conditions apply. Dave's unanimous
sentries can bypass the wait; a zero period permits immediate acceptance. The guardian can foreclose before
stage, during the wait or after the wait while no acceptance has occurred. Foreclosure after acceptance does
not undo the accepted root. The configured block policy can delay observation, so an alert does not guarantee time to act.

For example, with stage in block 100 and a period of 20 blocks, period-based acceptance first becomes eligible
in block 120. If `safe` or `finalized` first exposes that stage when the executable chain head is already 122,
the wait has elapsed. Unanimous sentries can permit acceptance earlier; period zero can permit stage and
acceptance in one block. Polling, scans, retries and scheduling add to observation delay. Alert delivery, the
guardian decision, signing and transaction inclusion take more time. There is no universal `period - poll`
reaction window and no early `latest` alert in this delivery. The guardian can still foreclose after block 120
if no acceptance has occurred.

Foreclosure does not revoke an accepted root. Outputs with a valid proof under that root can still execute
after foreclosure, subject to the normal output checks and the rule that an index can execute only once.

Inspect the application and both roots. Assess guardian/foreclosure options for the deployment. The node does
not stage, accept or vote for the IOR result to force progress. A later healthy claim does not remove the
earlier fact. Use the epoch RPC methods to inspect `calculated_outputs_root` with `tx_buffer_data_block`,
`commitment` and `status`.

The conditions below describe observed facts, not a guaranteed period in which an operator can react.

| Condition | Action |
| --- | --- |
| `ior_settlement_withheld` | Compare both roots and investigate the guest declaration and local output evidence. Check epoch and join facts through the API. Do not infer current settlement state from this detection record. |
| `ior_winner_settlement_withheld` | Review the current Dave winner and guardian options. This node will not settle IOR; without another actor's settlement the next sealed tournament cannot start. |
| `ior_result_staged` | Inspect current chain state and guardian options. The recorded sentry and period values are a snapshot. Stage may already have been accepted by the time the record arrives. |
| `ior_result_accepted` | Treat the root as accepted. Inspect output indexes and proofs; foreclosure cannot revoke that root or undo an output already executed. Different local bytes remain unexecuted locally. |

## Stored output proofs

The node keeps the normal proof stored when each output's epoch was computed. That proof resolves to the
calculated root of that epoch. The contract requires the resolved root to be accepted. Acceptance of a
wrong declared root does not make a proof against the different calculated root valid.

A later healthy epoch can contain the old output in its cumulative tree and accept a different root. The
old proof still resolves to its original root; the node does not replace it with a proof against the later
root. Thus an IOR-epoch voucher can remain unusable through the stored proof even after a healthy acceptance.
This delivery does not add proof regeneration or rerun machine execution.

If a later epoch accepts exactly the original calculated root, the original proof can pass that root check.
For example, an input can correct the declaration without adding outputs. Other execution checks still apply.
An index already consumed on chain cannot execute again, even if a valid proof for the local bytes is later
available. A new proof does not undo an earlier execution of different bytes.

## Output execution observations

`OutputExecuted` gives an index and bytes. It does not identify the accepted cumulative root used as proof.
A later accepted invalid declaration can authorize different bytes at an earlier local output index. This can
happen before local validation discovers IOR.

For a known local index with different bytes, the EVM Reader records a diagnostic and continues the successful
scan. It leaves application health unchanged and does not mark that different local output as executed. Matching
outputs in the same batch keep their normal execution update. The API can therefore show the local output
as unexecuted while the chain bitmap says that its index is consumed. Those local bytes cannot execute at
that consumed index. Admissible claim comparisons still detect a machine-hash or declared-root disagreement
when sufficient local result evidence and an eligible reconciliation path are available. The event diagnostic
does not replace detection for epochs that are never compared, or for later damage to local raw output bytes.

The following limits remain:

- The scanner starts this search only when a local voucher is pending. Events outside an eligible scan
  do not necessarily receive this diagnostic, even if an accepted IOR root permits their execution.
- An executed index absent from the local output table prevents this application's execution-event cursor
  from advancing. If that index never exists locally, later execution records for this application can stay
  unprocessed permanently, even with application status `OK`. Other applications have their own scan progress.
- A skipped mismatch leaves the local executed-output count below the chain counter. Later scans can perform
  extra first-block lookups. This delivery does not add an execution-event counter or replace that scanner.
- An absent local index or failed cursor write can cause a mismatch event to be read again. The named
  `output_execution_mismatch` WARN contains the index, lengths and keccak hashes instead of full payloads.

## Separate work

Known malformed automatic Output/Report continuation is not implemented here. Those paths can still stop under
`FAILED` without a join. New simultaneous occurrence inventory is also outside this delivery. Legacy manual
accepted-root length handling, sentry sending, dispute moves and snapshot redesign remain separate work.
