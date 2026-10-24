<!-- (c) Cartesi and individual authors (see AUTHORS) -->
<!-- SPDX-License-Identifier: Apache-2.0 (see LICENSE) -->

# Transaction delivery

The node submits transactions to advance application consensus and recover its
PRT bonds. Claimer handles Authority and Quorum. PRT handles DaveConsensus.
Each role has its own signer and sender. Within each role, an observer and a
publisher run independently.

The observer stores protocol facts in PostgreSQL, using `finalized` by default. The publisher reads
those committed facts and checks current contract state before sending. A slow
observation does not stop checks of a transaction already being published.
RPC acceptance does not prove that an action executed or changed protocol state.

The database stores no signed transactions or delivery journal. After a restart,
the node reconstructs useful work from protocol facts and contract state. This
design supports new deployments. This pre-release changes the initial database
schema to retain the epoch where execution or root validation stopped. Use a
fresh database and re-ingest the chain when upgrading an existing deployment;
there is no migration or inferred cutoff for an already populated database.
Changes to a database's registered chain, observation policy, submission mode
or signer are not supported.

## Prepare a deployment

- Set `CARTESI_BLOCKCHAIN_DEFAULT_BLOCK` to `latest`, `safe` or `finalized`.
  The node defaults to `finalized`. All contributing services must use the same
  policy. The provisional modes, `latest` and `safe`, have no rollback after a
  reorg: stored inputs or protocol facts can become invalid, and an operator may
  need to stop the node and rebuild its database. A rebuild cannot undo a claim
  or bond payment that already executed. Startup reports the lack of rollback.
  The sender keeps only its current transaction and releases that record when the
  latest mined nonce advances. It does this for every observation policy. `latest`
  makes no mandatory finalized read. `pending` is unsupported.
- Give Claimer and PRT different, code-free EOA addresses. Do not attach pending
  EIP-7702 authorizations to these accounts.
- Run one submitting process per signer. Independent deployments do not
  coordinate through their databases. Do not send from an active node's signer
  with the CLI or another wallet. If another program accidentally uses the
  wallet, the node reconciles nonce changes and retries at its polling interval.
  Lost-publication races with advancing nonces produce rate-limited warnings. A warning does
  not prove another program is using the signer. Stopping the competing program
  does not withdraw transactions it already sent. Those can keep occupying the
  nonce until mined, replaced or dropped from the provider's pool. Stop the submitting service before using
  a wallet to replace a pending transaction. Inclusion is not guaranteed during
  continued competition.
- Use one node deployment per database and one active process per service role.
  A split deployment runs its different service roles against that database.
  Stop the current processes completely before starting replacements. Use a
  stop-first rollout, for example Kubernetes
  [Recreate](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#recreate-deployment).
- Use a provider that supports canonical block-hash account reads and contract
  calls through EIP-1898, and gas estimation at a block number.
  Estimation sizes gas; exact hash-pinned simulation authorizes publication.
  Use one consistent RPC endpoint for the contributing
  services; its replicas must give coherent state and log results.
  It must serve state at the selected observation head
  and historical state, logs and headers for catch-up. Open Claimer epochs need
  state at their last processed block, even when the policy is `latest`. Retention
  must cover the oldest unreconciled epoch or cursor, including downtime and
  computation backlog. Verify the provider's actual query support; there is no
  universal block-count cutoff. PRT counter baselines
  come from committed database events; they do not require state at an old cursor.
  Input scans and Claimer observation can still require older contract state.
  In particular, Claimer reads accepted-claim counts at an open claim's epoch end;
  a provider that prunes that state can stop observation until archival state is available.

## Use a development chain

Development and local environments use `latest` because their chains do not
reorganize. Both integration targets accept `DEFAULT_BLOCK=latest`, `safe` or
`finalized`. The default is `latest`; an inherited
`CARTESI_BLOCKCHAIN_DEFAULT_BLOCK` is preserved unless `DEFAULT_BLOCK` is explicit.
The selector applies the same observation policy to all contributing services:

```sh
make integration-test-local DEFAULT_BLOCK=finalized
make integration-test-with-compose DEFAULT_BLOCK=finalized
```

`SHARD` and `NODE_TOPOLOGY` keep selecting the tests and deployment topology.
Local tests use your running Anvil with its existing finality settings. Compose
tests with `finalized` use a 64-block finality lag; other policies use the ordinary
devnet settings. CI adds two finalized cells that select three delivery suites:

```sh
make integration-test-with-compose DEFAULT_BLOCK=finalized SHARD=delivery-finalized NODE_TOPOLOGY=all
```

The focused profile stays within the normal job budget. Selecting every suite
with `finalized` takes longer and can exceed that budget. Compose CI cells that
select the basic standalone suites also run the short-deadline delivery fixtures.
On the host, run those separately with `make delivery-test`.

`eval "$(make env)"` exports the development settings directly. Use
`eval "$(make env DEFAULT_BLOCK=finalized)"` to select finalized observation for
other local commands. Both development Compose files also default to `latest`
and accept the full node setting as an environment override.

Use a fresh database when changing policy; changing a database's saved policy
is unsupported. The node's unconfigured default remains `finalized`. The selected policy controls stored protocol facts. Publication checks and
in-memory transaction retirement use latest mined state in all three modes.

## Connect PostgreSQL

The observer and publisher use the ordinary repository pool.
The database coordinates protocol facts within one deployment; it does not
coordinate wallets used by independent deployments.

Repository operations used by delivery have a two-minute client deadline,
including connection acquisition and database work. A shorter caller deadline
takes precedence. Deletion respects the CLI's explicit timeout and has a short
lock-wait limit. A failed or timed-out operation leaves the affected work for a
later polling iteration. Shutdown cancels outstanding work.

Configure PostgreSQL's
[idle transaction timeout](https://www.postgresql.org/docs/17/runtime-config-client.html#GUC-IDLE-IN-TRANSACTION-SESSION-TIMEOUT)
and [TCP dead-connection detection](https://www.postgresql.org/docs/17/runtime-config-connection.html#GUC-TCP-KEEPALIVES-IDLE)
for the node's connections. A client deadline cannot guarantee that a partitioned
server receives cancellation or releases transaction locks promptly. The idle
transaction timeout covers idle transactions; active statements need an
appropriate server-side statement timeout. Allow enough time for expected
observation commits and deletion work when choosing these server limits.

A large PRT observation interval can hold its application's row for seconds.
The EVM Reader and application removal use the same row. Removal can report a
lock timeout while observation commits; retry it after that transaction finishes.
Each refresh pass can repeat these locks and database writes; this cost is not
limited to discovering a new tournament.

## Publish a transaction

Each publisher asks its planner for **Publish**, **Wait**, or **Cancel**:

- Publish supplies a useful protocol action and its transaction request.
- Wait keeps the current transaction while a read or protocol waiting period is incomplete.
- Cancel requires positive evidence that the current action is obsolete. It asks
  for a zero-value self-transfer at that nonce. It cannot revoke an already sent transaction.

The sender reads one coherent latest block and the signer's mined nonce. It
checks the service publication guard, obtains a fresh fee quote, estimates gas at that block number,
and simulates the exact
sender, target, calldata, value, gas and fees at that block hash. Bond recovery
also has to return true. A successful gas estimate alone does not authorize a send.

After signing, the sender checks the service guard and current eligibility again. A
normal new block does not require another signature. A changed nonce, orphaned
preparation block, failed read or obsolete action prevents that POST. A failed
read does not authorize cancelling or replacing unrelated useful work.

The sender stores signed bytes before POST. It reuses a valid current envelope
when a fresh quote cannot clear its replacement threshold. It has no application, epoch, deadline or consensus rules;
the Claimer and PRT planners own those decisions. Each role keeps its own
publisher loop and in-memory sender.

Each HTTP request and each signing operation is bounded by
`CARTESI_BLOCKCHAIN_HTTP_REQUEST_TIMEOUT`. KMS HTTP requests also use this timeout.
The KMS request receives the caller's deadline and cancellation directly.
A pass can contain many requests and take longer than this timeout. Its finite
candidate budget limits work, not elapsed time. Each delivery database operation
has its own client deadline and also receives service cancellation. These limits
do not impose one deadline on an entire observation or publication pass.

## Choose a fee ceiling

`CARTESI_BLOCKCHAIN_TX_MAX_FEE_PER_GAS` is optional. Its value is a positive decimal
integer in wei. It caps dynamic `maxFeePerGas` and legacy `gasPrice`. Without it,
there is no operator price ceiling. It does not bound total expenditure.

For dynamic transactions, the sender uses a fresh quoted tip and a cap based on
the current base fee. It clamps the cap to the optional ceiling and the tip to
that cap. After a price refusal, it retries once with the tip equal to the actual
cap, without increasing the cap. This fallback can pay the full cap as the
price per gas, rather than the usual base fee plus quoted tip.

Legacy mode uses a fresh suggested gas price. Its one price-refusal retry uses
the greater of a ten-percent increase, rounded up, or one additional wei, then
applies the optional ceiling.

For the same current action, the sender reuses valid bytes while a fresh quote
cannot increase both fee fields enough to replace them. For cancellation or a
different selected action, it raises the quote to at least the cached bid's
ten-percent replacement step, with a strict increase of at least one wei.
The optional ceiling still applies. Once that replacement is retained, ordinary
polling reuses it; polling alone does not keep increasing the bid.

If that replacement step exceeds the ceiling, the sender tries the fresh capped
quote. The earlier copy might have left the pool. A definite refusal preserves
the old record. This can restore progress after eviction, but cannot force a
surviving older bid out of the pool above the configured ceiling.
While that copy remains, a pass can make two refused attempts and two signing
requests. No transaction fee is spent for a refused POST, but KMS requests can cost money.

A cached cap must cover the current base fee plus the smaller of the cached tip
and the fresh quoted tip. Merely covering the base fee can leave a zero effective
tip. In that case, or when cached gas is insufficient, the sender repairs the
envelope using the same bounded replacement step.

Retained bytes do not prove that the pool admitted them. After a definite
replacement-underpriced reply for the same action, the one retry uses the fresh
quote, including its full-cap tip. This can displace an unknown older bid after
restart and a lost reply. An ambiguous or mixed transport failure keeps its
uncertain result and does not authorize this price retry.
In legacy mode on a base-fee chain, a retained gas price must exceed the base fee.
A full-cap retry's tip also sets the minimum tip for later replacements at that nonce.

These rules do not guarantee replacing an arbitrary older transaction. If the
fresh quote cannot clear a provider's replacement threshold, the old transaction
can remain pending. Check the stalled-nonce warning's reply and the INFO
`Transaction publication` entry for that nonce. Its `fee_cap` and `tip` show the
attempted quote. Compare them with the configured ceiling. The operator may need
to fund the signer, increase the ceiling or use another provider.
Lowering the ceiling cannot cancel an already sent transaction or reduce its price.
Private relays need a verified release policy; public-pool progress assumptions
do not establish that every relay will release obsolete work.

These replacement steps target the supported pool profile; another provider can
require a larger bump. A fresh market quote can exceed the minimum step.
There is no reserved cancellation fee headroom or wallet reservation.
The sender checks the current
transaction's maximum gas cost plus value against the current balance. A join
uses the contract's required bond; there is no configurable join-bond cap.
The funding check before estimation uses only a minimum gas allowance. Its
reported amount is a lower bound; the final padded allowance can require more funds.

Ordinary contract calls add the greater of fifty percent or 150,000 gas to the
estimate. A value-bearing join adds the greater of the full estimate or 150,000
gas, to cover the pairing branch after a peer joins first. An action can supply
a minimum gas allowance, such as bond recovery with its payment callback.
The estimate can raise that allowance to cover chain-specific posting costs.
A neutral self-transfer uses its estimated gas without the contract-call padding.
An unavailable estimator can reuse the same retained request's gas only when its
exact simulation and fresh checks succeed. New or changed work, including a new
cancellation, waits for estimation to recover. Known estimator refusals retain
their cause and are reported as `gas estimate refused`. Check the named contract
condition, gas allowance, signer balance or provider capability. A transport
failure is reported as `gas estimate unavailable`. The sender does not invent a
large fallback allowance.
The sender bounds gas by the
block limit and its supported transaction limit. Unused gas is not normally
charged, but the larger limit increases the balance needed before inclusion.

## Understand retries and restarts

There is one preferred current-nonce transaction in memory. There is no signed
transaction journal, inventory of past nonces, receipt accounting pipeline or
cumulative application revert breaker.

A lost reply leaves the outcome unknown. The sender keeps the preferred bytes
for a later pass. If a changed replacement is definitely refused, it preserves
the previous usable request. An ambiguous replacement can coexist with an older
transaction in the network; the cache does not list every distributed version.

Latest mined nonce progress releases the cache, even when observation uses
`safe` or `finalized`. Inclusion consumes a nonce even when execution reverts.
The observer independently records protocol results at the selected policy.
RPC acceptance, an already-known reply and nonce progress do not prove that the
intended protocol action succeeded.

Restart loses the cache. A reorg can restore a consumed nonce after its bytes
have been discarded. The planner reconstructs useful work from computed data
and current contract state. An older higher-nonce copy can execute after the
lower gap is filled. Duplicate or losing-race execution can cost gas; there is
no historical classifier or automatic cumulative spending stop.
The cost depends on the action, calldata and chain gas rules; there is no fixed
gas charge for every duplicate claim.

After a restart with a transaction still pending, replacement-underpriced replies
can repeat until that copy is mined or a fresh quote clears its replacement threshold.
After a reorg to a shorter chain, publication can wait for the head to reach the
earlier preparation height. A failed read does not justify abandoning that record.

A known obsolete current action triggers best-effort cancellation. A later useful
urgent action can replace that neutral request. Neither can unsend old bytes:
a value-carrying join can still win the race and transfer its bond after the
application is disabled or foreclosed. Treat this as residual publication risk,
not a cancellation guarantee.

A provisional reorg can remove the tournament's deployment after join bytes
were sent. If those bytes execute on a branch without its code, the bond becomes
an ordinary ETH transfer: no join is recorded and recovery is not guaranteed.
The address might also contain a different tournament after reordered deployment.
A database rebuild cannot undo either payment. Observing creation at `finalized`
avoids this under the finality assumption; `latest` adds no hidden finalized read.

With `latest` or `safe`, a recorded submission can disappear after a reorg.
The publisher can re-submit the same locally computed claim when current contract
state permits it. The observer refreshes matching canonical transaction and staging
metadata. This repairs delivery of that intent; it does not verify an orphaned input
history. Stored ACCEPTED rows, cursors or terminal health facts can still require a rebuild.
Fresh work is not rediscovered from ACCEPTED rows.

Claimer uses one event-scan path to reconcile submissions and acceptance. It keeps
event transaction hashes for audit. Observation can wait for an application's next
page turn and require more event reads than a receipt shortcut. External acceptance
out of epoch order is valid; the observer identifies the epoch before comparing roots.

Before acceptance, a submitting Quorum node records its own matching vote hash,
separately from the majority's staging block. A replacement canonical own vote can
repair that link. If no own vote survives, the hash can be absent even when peers
stage the claim. A reader without a local signer can record the staging event's
hash. After acceptance, the field can identify the acceptance transaction instead.
A Quorum vote cannot be withdrawn. A divergent result saved from `latest` or
`safe` is also provisional; it can require a database rebuild after a reorg.

## Keep applications progressing

Claim discovery rotates applications by ID, skipping applications without a row
in the requested claim source. Each application has its own epoch
position; another application's epoch numbers do not determine its turn. Each
turn inspects the oldest relevant row and a bounded later window before choosing
work. A waiting oldest row cannot hide all later work, and a busy application
moves behind the next application after its turn.

A ready acceptance takes priority within the visited work. This is not a global
ordering across all applications. The sixteen-candidate pass budget bounds
planning attempts, not RPC count, inclusion delay or unlimited backlog capacity.
New arrivals do not reset the positions of applications already being visited.

A failed preflight candidate gets a bounded cooldown and the publisher can try
another candidate. Normal protocol maturity waits stay quiet. Persistent named
refusals report the application and cause. A contract race after simulation can
still revert on chain and cost gas. The publisher has no application-wide
cumulative execution stop and provides no total spending guarantee.

A useful current action keeps its nonce while simulation fails or required funds
are absent. Other applications on that role can wait behind it. A failed read
does not prove the action obsolete. PRT can still choose a verified earlier-deadline
join according to its priority rule.

PRT discovers its bounded candidate list once per publication pass. A refused
preparation consumes that candidate's turn. Every selected action still receives
fresh protected validation before POST, including after a head change.

## Size observation and deadline capacity

Use the existing role polling interval, HTTP request timeout and
`CARTESI_BLOCKCHAIN_MAX_BLOCK_RANGE`. There are no delivery pass, candidate,
observation or urgency timeout settings.

Each role normally starts a new nonce only after it observes the previous one
mined. Capacity is therefore limited by inclusion and polling. Rebroadcasts can
make several POSTs in one block; they do not increase useful nonce throughput.
On a chain with 12-second blocks, the ideal ceiling is 300 transactions per hour
per role, before polling, waiting, recovery and failed execution costs. A claim
can need submission and acceptance; a PRT epoch can also need a join and bond
recovery. Size the total action load across all applications below that capacity.
Applications with pending but immature work still take turns in claim rotation.

PRT advances each tournament from its own saved block. When it discovers a new
child, it scans that child and any new descendants through the same endpoint.
That complete unit commits atomically. An existing child advances independently.
A failed tournament read does not roll back another tournament's completed unit.
The application's aggregate tournament height is diagnostic, not complete coverage.

One dense tournament or newly discovered subtree can require substantial RPC,
memory and database work before its atomic observation commits. There is no internal
node-count or subtree-size ceiling. SQL statements are chunked within that transaction;
chunking does not cap its total work. Other known tournaments receive separate turns,
but a slow unit can delay the observer's next turn. The sender keeps one current transaction rather than an observation-history inventory.

PRT can replace current protocol work with a ready join that has a strictly earlier
finite deadline. An untimed action has no deadline. The planner owns this choice;
there is no one-use takeover allowance or sticky neutral state. Replacement still
depends on current evidence, funds, provider policy and fees.

Measure observation lag, provider latency, signing time and inclusion time against
the deployed tournament allowance. Independent loops and urgent replacement do not
guarantee meeting a deadline.

## Execution stops and integrity findings

An execution stop (`GUEST_EXCEPTION`, `MACHINE_HALTED`, `MCYCLE_OVERFLOW` or
`UNEXPECTED_YIELD`) or `INVALID_OUTPUTS_ROOT` at epoch P stops further execution.
An enabled application can still validate and complete a valid, complete epoch
before P: Authority/Quorum submit or accept, and PRT join, stage or accept.
An invalid root found in an earlier epoch moves P back, including after restart.
The earlier work still needs valid proofs, signer authorization, time for
inclusion, and all foreclosure and protocol checks. This does not provide active
dispute defense or discovery of every other party's claim.

With `finalized` observation, a stored `CLAIM_STAGED` row that reads `UNSTAGED`
once the latest head reaches its recorded staging block causes publication to
wait. A warning identifies the application, epoch, recorded staging block and
latest block, at most once per minute for each application and epoch. The latest
read does not itself establish finality. Check that the provider serves coherent
views of the registered chain. A held transaction keeps its signer occupied;
other applications sharing that signer can also wait. The warning does not
cancel or replace that transaction.

The observer checks for this contradiction after the configured view reaches the
staging block plus the application's staging period, and only in submitting mode.
If it sees the contradiction then, a healthy application becomes `FAILED`.
Restore a coherent endpoint for the registered chain before re-enabling. If the
persisted history no longer matches that chain, rebuild and re-ingest into a
fresh database; do not edit individual saved facts to force progress.
An application already stopped by execution or an invalid root becomes
`CORRUPTED`, which also stops foreclosure drain. Re-enabling cannot clear
`CORRUPTED`: correct the cause and re-ingest into a fresh database.
Recorded foreclosure can instead terminalize the claim as `CLAIM_FORECLOSED`
before this diagnosis. If the views disagree, the observer may not see the same
contradiction. There is no guaranteed diagnosis deadline.

The new integrity reason replaces the earlier reason. The database retains the
earliest known stop's typed epoch boundary in `application.problem_epoch_index`,
`problem_kind` and `problem_input_index`; logs provide further context if retained.
Those columns are not separate JSON-RPC response fields. The current reason is
not a full history of previous diagnostics.

With `latest` or `safe`, the publisher can repair a vanished stage by resubmitting.
A claim recorded only as `CLAIM_SUBMITTED` has no persisted staging fact and
remains eligible for that repair even with `finalized`. Transactions distributed
earlier, independent actors and changes after the pre-send check can still affect
the chain.

An unknown consensus-contract claim status is refused. A healthy application can
become `FAILED`; an application stopped at P keeps its status. If a transaction
is already held, this refusal can also hold up other applications on the signer.
Restore a supported contract/provider and inspect the held transaction; restarting
does not revoke already distributed bytes. If the validator cannot build a PRT
commitment proof under a stop, it retries the epoch, which can keep foreclosure
drain pending.

The node may not discover other parties' claims at or after P. After foreclosure,
Authority/Quorum can mark those epochs `CLAIM_FORECLOSED` even if a foreign claim
was accepted. PRT leaves an epoch pending when it has seen a claim transaction
but has no validated local claim to compare, so drain can remain pending.
Neither case authorizes publication of that epoch. Earlier validated epochs
retain their normal reconciliation.

Known limitations:

- An `OutputExecuted` event names an output index and bytes, but not the accepted
  cumulative root proving them. After an execution stop, different bytes can
  cause a false `CORRUPTED` diagnosis when a later invalid root proves them.
  This can also happen after an earlier invalid-root finding has moved P back,
  because the application status may still name the execution stop.
- A machine can be released before a closed epoch of a foreclosed application
  has been finalized.
- The output reader can stall on an executed output index that is absent locally.

## Disable and remove an application

Disable stops observation, fresh action discovery, useful replacements and bond
recovery at the next iteration. Work already in progress can finish. A transaction
already sent can still execute. Neutral nonce maintenance can still spend fees.
Disabling also stops protection alerts.
`FAILED`, `DIVERGED` and `CORRUPTED` prevent useful publication and can trigger
best-effort cancellation of current work, including when `FAILED` is recoverable.
A missing stop boundary or a failed read is inconclusive: that evidence alone
cannot justify cancelling or replacing retained work. An independently verified
priority replacement still follows the planner's normal rules.
PRT bond recovery is an exception: an enabled application's recoverable bond can
still be returned when its on-chain payee is the configured signer, even when the
application's health is not OK. Disabling the application stops this recovery too.

Removal requires a disabled application and `--allow-abandonment`. This flag
acknowledges that removal ends protection and alerts, deletes the stored state,
and abandons outstanding work. `--yes` skips only the confirmation prompt.
The command does not scan the blockchain or classify outstanding obligations.
Normal observers, transaction preflight and bond recovery keep their own chain
reads; this simplification affects only application removal.

```sh
cartesi-rollups-cli app remove my-app --allow-abandonment
cartesi-rollups-cli app remove my-app --allow-abandonment --yes --timeout 10m
```

The timeout bounds deletion, including lock acquisition. The repository checks
that the application is still disabled when it deletes the row. Observation
transactions preserve their application-row locks and atomic cursor updates.
Removal cannot revoke transactions already sent or in progress; those may still
execute. Check outstanding claims and bonds before deliberately abandoning them.

Retain any required tournament and payee identities for later operator recovery.
Anyone can recover a bond to its on-chain payee. Automatic recovery uses only the
configured signer. The node has no signer registry or manual-recovery CLI.
For a join that executes after removal, the tournament's permissionless
`tryRecoveringBond()` method pays its recorded recipient once the bond is
recoverable. An operator must initiate this recovery after removal.

The former `CARTESI_CLAIMER_MAX_ACCEPT_ATTEMPTS` setting has been removed.
It is ignored; there is no cumulative application revert breaker.

## Respond to operational signals

| Signal | Action |
| --- | --- |
| Waiting for observer registration; service unready | Restore database access or start the contributing observer with compatible chain and observation settings; the service retries |
| Protected RPC read failed or unsupported | Restore a compatible provider; the next iteration retries protected reads |
| Stalled nonce or frozen head | Check provider progress, fees and inclusion |
| Named gas, balance or price limit | Inspect the required and available amounts |
| Claim publication waiting for funding or a policy limit | Check the named gas, balance or price limit. A funding amount before estimation is a lower bound |
| Same action still needed after nonce advancement | Check contract state and accidental wallet sharing. This symptom does not prove another publisher exists or that our transaction reverted |
| Claim or PRT publication lost a race while signer nonce advanced | Check other users of this signer and provider consistency; stop competing senders and inspect pending transactions |
| Replacement underpriced for a new action | A pre-restart or foreign transaction can already occupy the nonce. Stopping its sender does not remove it; wait for inclusion or replace it with the submitting service stopped |
| Gas estimate unavailable | Restore the estimator; new work waits, while suitable retained bytes can still be simulated and reused |
| Gas estimate refused | Check the returned cause: contract state, gas allowance, signer balance or provider support. This is not necessarily an estimator outage |
| Join near its deadline | Check observation lag, backlog, signing and inclusion time |
| Claim candidate refused; retrying after cooldown | Check the named signer against the Authority owner or Quorum validator membership |
| Claim or PRT publication deferred | Check the named cause; recoverable failures produce rate-limited warnings and retry |
| PRT transaction could not publish; planning will retry | Check the named action and cause. For a join, compare the remaining deadline with signing and inclusion time |
| PRT application action could not be planned; other applications can continue | Check the named application's read or planning failure; other applications retain their turns |
| PRT cannot publish consensus action with the registered contracts | Check the named contract link or geometry; staging and acceptance for that application are paused |
| Action prefixed with "cancel obsolete" | The node is trying to replace a sent action; check funds, the fee ceiling and pool replacement policy |
| PRT cannot join a tournament | Check the named geometry, registration link or missed join window; protection for that epoch is unavailable |
| Recorded submission or provisional staging is absent from the current chain | Check provider consistency or a provisional reorg; same-claim delivery can recover, but invalidated inputs require a rebuild |
| Input observation is inconclusive | The count and logs disagree; the cursor stays unchanged and the next poll retries. Restore a coherent provider if failures persist |
| Finality or call-content contradiction | Stop and investigate; do not edit saved facts to force progress |
| Claim publication waiting: recorded staging contradicts latest view | Check endpoint/chain consistency and compare the named blocks. The observer can wait for staging maturity; a held transaction also holds the shared signer |
| Application FAILED: getClaim returned UNSTAGED for recorded CLAIM_STAGED | Restore the correct coherent endpoint before re-enabling. If saved history no longer matches the chain, re-ingest into a fresh database |
| Application CORRUPTED: getClaim returned UNSTAGED for recorded CLAIM_STAGED | Correct the cause and re-ingest into a fresh database. Re-enabling cannot clear CORRUPTED |

Alert on sustained publication-deferred, could-not-publish, waiting-for-funding,
stalled-nonce or frozen-head warnings, and on join-deadline or cannot-join warnings.
A single recovered read failure or normal shutdown cancellation does not need an
operator alert. Readiness alone does not detect every persistent publication
refusal.

A protected-state contradiction stops the affected publisher and makes it
unready. The process stays up so it can report the fault. The stop is held in
memory; a restart clears it. First verify the provider and saved evidence, because
restarting does not repair a real contradiction.

With `latest` or `safe`, a reorg can remove a saved claim or tournament fact.
Saved cursors can then be ahead of the canonical chain. A provider disagreement
can cause similar symptoms, so check the canonical chain first. If stored facts
came from an orphaned branch, stop the node and rebuild its database before
resuming submission. Do not edit individual facts or the registered policy to
force progress.

Readiness reports hard prerequisites. It does not prove that every application is
progressing or that a deadline will be met. Monitor the progress warnings as well.
A near-deadline join warning requires an identified current join. Before a root
has complete committed observation, planning can wait at Debug level without
that warning. Monitor epoch and tournament observation progress separately.
The sender reads no receipts: it cannot report each included revert or its gas
cost. A repeated-action warning is only a symptom that the protocol still needs
that action after nonce advancement.
