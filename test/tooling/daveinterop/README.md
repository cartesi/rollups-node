# daveinterop — rollups node and Sling node interoperability

`daveinterop` is a development tool, not part of a release build. It checks
that two PRT implementations agree on the same application:

- the **rollups node**: this repository (`cartesi-rollups-node`);
- the **Sling node**: the reference PRT node of the Dave repository
  (`cartesi-rollups-prt-node`).

"Dave" names only the Dave repository, its test harness
(`just rollups-tests::test`), and its contracts (`DaveConsensus`).

Plan: `docs/dave-interop-testing-plan.md` (revision 6, phases P1 and P1b).

## Quick start

```bash
eval $(make env)                # CARTESI_* settings
make start-postgres             # the tool creates one database per run
export DAVE_ROOT=/path/to/dave  # a prepared Dave tree (Sling node and program images built)
make interop-check ARGS="--for suite"
make interop-suite              # echo/simple and honeypot/stf_all: about 12 min the first time
```

`make interop-suite` builds the node and the tool first. The suite prints its
plan and time estimate, then one progress line per step. The harness part is
slow and silent, so the tool prints a line every 30 s:

```text
21:34:54 suite: 2 scenarios, about 12 min in total (finishing near 21:47):
21:34:54 suite:   echo/simple                        capture about 2 min (typical); replay about 1 min
21:34:54 suite:   honeypot/stf_all                   capture about 9 min (typical); replay about 1 min
21:34:54 ━━ [1/2] echo/simple, about 3 min ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
21:34:54 capture: running the Dave harness: just rollups-tests::test echo simple (log _interop/cases/echo-simple/harness.log)
21:34:54 capture: the harness plays a full Dave dispute on its own Anvil: about 2 min (typical); it logs nothing here, so a progress line follows every 30s
21:35:24 capture: harness running, 30s of about 2 min; last line: ...
...
21:37:12 verify: waiting for the rollups node (0s): inputs 0/7 processed, epochs 0/3 computed, tournaments 0/5 indexed, scanned to block 0 of 971
21:37:31 verify: PASS (computation agreement tested, tournament indexing tested)
21:37:31 result: echo/simple PASS after 2m37s
```

Later runs reuse the captured cases (about 30 s per scenario). The result goes
to stdout and the progress to stderr; `--json` prints the result as JSON, and
`-v` adds debug lines and streams the harness output.

## What the result means

Every check reports `pass` (✔), `fail` (✘), or `not-tested` (○), and a run
passes when no check fails.
A `live` run can also report a `finding` (⚑): a defect of the Sling node that
does not fail the run.
The text report uses colors on a terminal (`NO_COLOR` turns them off), wraps
at 110 columns (`COLUMNS` changes it), and `--json` gives the full report. Two
**coverage** lines say whether the run exercised the properties we care about
at all. The plan calls them C1 and C2:

| Coverage | Plan | Tested when |
| --- | --- | --- |
| computation agreement | C1 | At least one epoch matches the Sling node's evidence **and** changes the machine state. |
| tournament indexing | C2 | The rollups node indexed every sealed epoch, tournament, commitment, and match on chain. |

`honeypot` rejects the plain inputs of most harness scenarios, so its epochs
end at the state where they began. It cannot test computation agreement, and
its result says "not tested". `echo` accepts every input except input number 2,
so it tests agreement, and the suite requires it (`--require-agreement`).

The Sling node's evidence is its own, never the rollups node's:

- the claims in its stage log ("stage tournament result of epoch N with claim
  X"), captured with the case;
- its joins (commitment and final state) in each root tournament, and its
  sentry claims, both read from the chain. The harness runs it as test
  account 7.

A root tournament winner is compared too, but only as a divergence check: the
winner can be the rollups node's own commitment.

## Commands

| Command | What it does |
| --- | --- |
| `suite` | Captures (or reuses), replays, and verifies a list of scenarios without interaction. `--list` prints the sets. |
| `check --for capture\|replay\|live\|suite\|run-sling` | Reports the prerequisites of one capability. Installs nothing. |
| `capture` | Runs a Dave harness scenario and records its chain as a case directory. |
| `replay` | Loads a case into a test-owned Anvil, attaches a fresh rollups node, and verifies it. `--then` continues on the same chain. |
| `verify` | Compares a running rollups node with the chain and the Sling node's evidence. |
| `live` | Runs the rollups node and the Sling node together on a test-owned chain. |
| `run-sling` | Attaches the Sling node to an existing chain in the foreground, for manual sessions. |
| `clean` | Removes run directories and failed captures, keeps captured cases (unless `--cases`), and drops `interop_*` databases. |

`make daveinterop` builds `./daveinterop`; the `make interop-*` targets build
it first and pass `ARGS`:

| Make target | Command |
| --- | --- |
| `make interop-suite ARGS="..."` | `suite` (builds the node first) |
| `make interop-check ARGS="--for replay"` | `check` |
| `make interop-capture ARGS="..."` | `capture` |
| `make interop-replay ARGS="--case DIR"` | `replay` (builds the node first) |
| `make interop-live ARGS="--order rollups-first"` | `live` (builds the node first) |
| `make interop-verify ARGS="--app NAME ..."` | `verify` |
| `make interop-sling ARGS="..."` | `run-sling` |
| `make clean-interop` | removes all of `_interop/`, captured cases included; drops no database |

Exit codes: 0 pass, 1 a check or an operation failed, 2 invalid arguments,
3 a missing prerequisite. `run-sling` passes the Sling node's exit code
through. Make itself returns 2 for any failing command.

Defaults: the Postgres admin URL is `$CARTESI_DATABASE_CONNECTION`, or the
`make env` one. The Sling node binary is `$DAVE_NODE_BIN`, or
`$DAVE_ROOT/target/debug/cartesi-rollups-prt-node`.

## suite

```bash
make interop-suite                                         # smoke: echo/simple, honeypot/stf_all
make interop-suite ARGS="--scenarios all"                  # every in-scope pair: 21 scenarios, about 70 min
make interop-suite ARGS="--scenarios honeypot/stf_all --then rollups-continues-alone,output-execution"
make interop-suite ARGS="--list"
```

`all` includes two pairs that are not in Dave's justfile: `echo/stf_all`
(state-transition disputes on accepted inputs) and `honeypot/stf_revert`, which
replaces `yield/stf_revert` (honeypot also rejects every plain input, so the
dispute lands on the revert restore).

For each scenario, the suite:

1. reuses `_interop/cases/<program>-<scenario>` when it is golden, or runs
   the harness again into it (`--fresh` always captures, into a new
   time-stamped directory). A directory left by a failed capture is kept as
   `<dir>.failed-<time>`;
2. replays the case into a fresh rollups node and verifies it;
3. with `--then`, runs the continuations (`output-execution` only on
   honeypot cases) and verifies again.

It never stops at the first failure, and it exits 1 when any scenario failed.
The harness runs its Anvil on port 18555 (`--test-instance`), so the devnet on
8545 can keep running. A harness run has a 30 min deadline
(`--harness-timeout`). The database of a passing replay is dropped
(`--drop-db=false` keeps it).

A case is **golden** when this tool ran the harness and saw exit code 0, the
Sling node log of that run was captured, and the Sling evidence is consistent
(its stage claims equal the finished root winners, and it logged a claim for
every epoch it staged). The suite refuses a case that is not golden. When the
Dave tree has moved since a case was captured, reusing it logs a warning.

## capture

```bash
./daveinterop capture --dave-root "$DAVE_ROOT" --program honeypot --scenario stf_all \
  --out _interop/cases/honeypot-stf_all
```

| File | Content |
| --- | --- |
| `manifest.json` | Provenance (revisions, Sling binary checksum, harness exit code and where it comes from, harness time), chain facts, application, deployments, and reference facts |
| `anvil-state.json.zst` | The chain dump, compressed (887 MB → 16 MB for honeypot/stf_all) |
| `template/` | The program template the application was deployed with |
| `dave.log`, `harness.log` | The Sling node log and the harness log of the run |

To import a dump recorded by hand, add `--from-dump FILE --sling-log FILE
[--harness-log FILE]`. An imported case is never golden: nobody saw the
harness exit code.

## replay

```bash
./daveinterop replay --case _interop/cases/honeypot-stf_all
./daveinterop replay --case _interop/cases/honeypot-stf_all --then rollups-continues-alone,output-execution
```

`replay`:

1. checks the dump checksum, expands it into `runs/<time>/`, and starts Anvil
   with mining off. It waits until every historical state is readable, and
   checks the head, the input count, and the sealed and staged epochs against
   the manifest;
2. creates a new database (`interop_<case>_<time>`) and registers the
   application;
3. starts the rollups node (claim submission off, unless `--then`) and waits
   until it has indexed and computed everything, with the same state in two
   snapshots 10 s apart;
4. runs the checks below.

With `--then`, it mines 10 blocks per second, runs the continuations, stops
mining, and verifies again at the new head. The continuations need claim
submission, and the rollups node cannot change that on an existing database,
so `--then` turns it on from the start. This does not affect the first
verification: with mining off, nothing the node sends is mined before the
continuations start.

| Continuation | Time | Checks |
| --- | --- | --- |
| `rollups-continues-alone` | about 4 min | With the Sling node gone, the rollups node stages and accepts the last sealed epoch, then joins, stages, accepts, and recovers its own bond for the next one. Every transaction is checked for its sender. An epoch that another signer staged before the continuation is "not tested". The rollups node must not be paid for a bond it did not post. |
| `output-execution` | about 5 min | honeypot only: mint TestFungibleToken, deposit it through the portal, request a withdrawal from account 1, wait for acceptance, execute the voucher with the CLI, and check `OutputExecuted`, the token transfer, the balances, and the indexed execution. The tx-buffer word must equal an independently computed outputs root. |

The run directory keeps the logs and `report.json`. Large temporary files (the
expanded dump, the Anvil cache, template copies) are removed on stop, and the
machine snapshots when the run passes. `--keep` leaves Anvil and the node
running for inspection until Ctrl-C.

## Checks

`verify` reads the chain with the generated contract bindings, never with the
rollups node's adapters, and the rollups node through its JSON-RPC API. Every
comparison runs in both directions: the node may neither miss nor add
anything.

| Check | Pass condition |
| --- | --- |
| `application.status` | `OK` (or `--expect-status`) |
| `application.identity` | The node's application, consensus, and template equal the application contract's |
| `inputs` | Every `InputAdded` event is indexed with the same payload, block, transaction, log index, and epoch; all are processed |
| `epochs.sealed` | Every sealed epoch has the same bounds, last block, and tournament, and a computed commitment (or `CLAIM_FORECLOSED` for a foreclosed application); every other node epoch is the open one, with the right bounds |
| `epochs.staged` | Final state, outputs root, and block equal `EpochStaged`; every epoch the node calls staged is staged on chain |
| `epochs.accepted` | Every accepted epoch is `CLAIM_ACCEPTED` with the accepting transaction, and the reverse |
| `commitments.sling` | Commitments equal the Sling node's stage claims and joins. The Sling node logs its commitment only when it stages, and a commitment is joined only once, so an epoch that the rollups node joined first has no Sling commitment; the detail lists those epochs as not compared |
| `final_states.sling` | Final states equal the Sling node's joins and sentry claims |
| `commitments.root_winner` | Every finished root has the node's commitment as winner (divergence check); roots still running are listed |
| `tournaments` | Every tournament, with its parent and epoch |
| `tournaments.commitments` | Every `CommitmentJoined` event, with final state, submitter, block, and transaction |
| `tournaments.matches` | Every `MatchCreated` event, with its commitments, block, and transaction, and every `MatchDeleted` |
| `application.foreclosure` | The node's foreclosure block and transaction equal the `Foreclosure` event, every epoch never accepted is `CLAIM_FORECLOSED`, and the accounts-drive proof equals `AccountsDriveMerkleRootProved`; not tested when the application is not foreclosed |
| `withdrawals` | Every `Withdrawal` event is indexed with the same account, output, block, transaction, and log index, and the reverse |
| `agreement.state_change` | At least one epoch matches the Sling node (commitment or final state) and changes state; the detail lists the epochs that changed and those that did not (`--require-agreement` makes "not tested" a failure) |

The wait ends early when the application reaches another status than
expected, or when Anvil, the rollups node, or the Sling node exits.

`verify` also works on a running node, but it needs a chain that stands still:
the node must reach the block that the checks read. On the devnet, stop mining
first (`cast rpc anvil_setIntervalMining 0`), or pin the block with
`--at-block`:

```bash
./daveinterop verify --app <name> --rpc http://127.0.0.1:8545 --api http://127.0.0.1:10011/rpc \
  --sling-log <sling.log> --wait 5m
```

## live

```bash
./daveinterop live --order concurrent        # start both nodes before the inputs
./daveinterop live --order rollups-first     # start the Sling node after the rollups node joined
./daveinterop live --order sling-first       # start the rollups node after the Sling node joined
./daveinterop live --program honeypot        # echo is the default; --template DIR for any other
make interop-live-dapp                       # once, for the full scenario
./daveinterop live --scenario full           # five planned epochs, a fake commitment, every output
make erc20-withdrawal-dapp build             # once, for the foreclose scenario (and its machine tool)
./daveinterop live --scenario foreclose      # deposits, a foreclosure, and every token back
```

`live` starts Anvil from `$DAVE_ROOT/cartesi-rollups/contracts/state.json`,
deploys a PRT application with the rollups CLI (the Sling node's signer is its
only sentry, unless `--sling-sentry=false`), starts the nodes in the selected
order, sends inputs from account 3, waits until their epochs are accepted,
freezes the chain, and verifies. Only one participant can join a given
commitment, so a second join by the two nodes means that they computed
different commitments. Both nodes poll every second, so the races between them
(join, stage, accept) are fair. Waits log a line every 30 s.

`--scenario smoke` (the default) sends five inputs into one epoch.

`--scenario full` (about 10 min) uses the test dapp in `test/dapps/interop-live`
(`make interop-live-dapp` builds it into `applications/interop-live-dapp`). A
payload that starts with `reject` is rejected with a report; any other emits a
voucher (1 gwei to the sender, empty payload), a notice, and a report, and is
accepted. The scenario plans five epochs and sends each epoch's inputs while
it is open:

| Epoch | Inputs | Checks |
| --- | --- | --- |
| 1 | 3 accepted | statuses as planned; who joined, who accepted |
| 2 | 2 accepted, 2 rejected, interleaved | same |
| 3 | 3 rejected | same |
| 4 | none | same |
| 5 | 3 accepted, and a fake commitment from account 5 | the fake commitment joins against the honest one, loses its match, and the honest commitment wins the root |

Then test account 4 funds the application with the total value of its
vouchers, every voucher is executed with `cartesi-rollups-cli execute`
(checking `OutputExecuted` and that the rollups node records the execution),
and every notice is validated with `cartesi-rollups-cli validate`. Balances
are read before the funding and after the last execution, and every change
must be exact: the application ends where it started, the input sender gains
the vouchers' value, the funder loses the funding plus its gas, and the CLI
signer loses exactly the gas of its execute transactions (`gasUsed` times
`effectiveGasPrice`).

Bonds: for the root tournament of every epoch from 0 to 5, there must be one
`BondRecovered`, for the root winner, paid to the account that joined it. The
payment and the burned remainder must follow `Tournament.tryRecoveringBond`:
of the joins' bonds minus the gas refunds (`PartialBondRefund`), the winner's
joiner gets everything up to one bond, and one bond plus a tenth of the rest
above it; the rest is burned, and the tournament ends empty. The fake
commitment's bond therefore pays the defender's gas refunds, a tenth goes to
the winner, and the rest burns.

The outputs root of every staged epoch must equal the independently computed
root of the outputs so far. Last, the usual two-way verification runs at the
frozen head.

The fake commitment is Dave's `bad_commitment` recipe: all-zero states with a
valid proof. It never moves, so it should lose by timeout. The rollups node
does not play matches, so the Sling node must defend the honest commitment,
also when the rollups node joined it; the report says who joined it.

`--scenario foreclose` (about 5 min) uses the ERC-20 withdrawal dapp of
`make erc20-withdrawal-dapp` (`test/dapps/erc20-withdrawal`; it must be built
for the TestUsdc and ERC-20 portal of the devnet bundle), deployed with a
withdrawal config: guardian account 1, the bundle's
`TestUsdWithdrawalOutputBuilder`, and the accounts drive of the template. It
mints TestUsdc to depositors A (account 2), B (8) and C (9), then:

| Epoch | Inputs | After the foreclosure |
| --- | --- | --- |
| 0 | none | accepted |
| 1 | A deposits 100, B 200, C 300; A withdraws its 100 | accepted; A's voucher is executed, B and C withdraw from the accounts drive |
| 2 | B deposits 20, C 30 | sealed, never accepted; both deposits are refunded |

The guardian forecloses the application as soon as epoch 2 is sealed. Then:

- a deposit after the foreclosure must revert with `ApplicationForeclosed`;
- A's voucher is executed, and the rollups node must record the execution;
- `cartesi-rollups-machine-tool replay --to-epoch 1` rebuilds the machine of
  epoch 1, whose root must equal the node's epoch 1 machine hash and
  `getLastFinalizedMachineMerkleRoot`; `prove accounts-drive` proves B and C,
  and must find no account for A, who withdrew everything;
- `cartesi-rollups-cli prove-drive-root` proves the drive root (a second proof
  must revert with `AccountsDriveMerkleRootAlreadyProved`), and B and C
  `withdraw` (a second withdrawal must revert with
  `AccountFundsAlreadyWithdrawn`); the rollups node must index both;
- `cartesi-rollups-cli refund` refunds inputs 4 and 5; refunding epoch 1's
  input 0 must revert with `CannotRefundFinalizedInput`, and refunding input 4
  again with `RefundAlreadyIssued`;
- every depositor must end with its TestUsdc balance after the mint, and the
  application with none;
- the rollups node must close epoch 2 (sealed, never accepted) and epoch 3
  as `CLAIM_FORECLOSED`. Epoch 3 opened when epoch 2 was sealed and was still
  open, with no inputs, at the foreclosure; it can never be sealed.

Epoch 2's tournament goes on after the foreclosure: the rollups node does not
join it, but the Sling node does. The test waits until it ends (about a
minute), because only then do the nodes try to settle the epoch, which the
foreclosure forbids, and only then must its joiner get the bond back. After
20 more seconds, the chain is frozen, and every transaction that each node
sent after the foreclosure is listed with its revert reason. A reverted transaction of the rollups node
fails the run. Reverted transactions of the Sling node are reported with the
status `finding` and do not fail the run: the Sling node does not handle
foreclosure and keeps sending settlement calls that revert, which should be
reported to the Dave team. The bonds of epochs 0 to 2 and the usual two-way
verification follow; it accepts `CLAIM_FORECLOSED` for a foreclosed
application and checks the foreclosure, the drive proof, and the withdrawals
that the rollups node indexed.

The `live` report starts with the verdict and the count of checks, then an
epochs table (the plan, the inputs as the rollups node processed them, who
joined, the epoch on chain, and the rollups node's status; a planned epoch is
checked against its plan), the scenario's sections, and the verification.
Findings for the Dave team come last.

Signers on the test chain: CLI and deployer 0, guardian 1, depositor A 2,
input sender 3, funder 4, fake commitment 5, rollups node PRT 6, Sling node 7,
depositors B 8 and C 9.

## Manual session on the devnet

```bash
make start                                      # devnet on 8545 and Postgres
eval $(make env)
./cartesi-rollups-cli deploy application echo-live "$DAVE_ROOT/test/programs/echo/machine-image" \
  --prt --claim-staging-period 50 --sentries 0x14dC79964da2C08b23698B3D3cc7Ca32193d9955 --json
CARTESI_FEATURE_CLAIM_SUBMISSION_ENABLED=false ./cartesi-rollups-node    # terminal 1 (observer)
./daveinterop run-sling --app <address> \
  --template "$DAVE_ROOT/test/programs/echo/machine-image" \
  --state-dir /tmp/sling-state --mnemonic-index 7 2> >(tee sling.log >&2)  # terminal 2
./cartesi-rollups-cli send echo-live 0x68656c6c6f --hex --yes           # inputs
```

`run-sling` writes the derived key to `<state-dir>/signer.key` (mode 0600);
the key never appears on a command line. The devnet mines one block per
second, so dispute clocks keep running while you read logs.

## Scope

- The guest must write the root of all outputs emitted so far into the
  tx-buffer word of every accepted state, including the template. Dave's
  `yield` program does not, so the tool refuses it. Use `echo` and `honeypot`.

## Notes

- **Anvil 1.5.1 opens its RPC port before a dump's history is loaded, and
  loads the historical states in no particular order.** The tool reads every
  block below the head and waits until all are readable. It fails when the
  set stops shrinking for 30 s: the dump lacks those states.
- **Anvil drops the history of a loaded dump when it cannot write
  `$HOME/.foundry/anvil/tmp`**, without an error. Every Anvil that the tool
  starts gets a private `HOME` in its run directory.
- **Default Anvil retention is too small.** The rollups node reads counters
  back to the deployment block, for example when the first output is
  executed. After a few thousand mined blocks, Anvil had evicted that state
  and the execution was never indexed. The tool passes
  `--max-persisted-states 20000` (`--anvil-persisted-states`), and the miner
  stops with an error before the chain outgrows it. Each state takes about
  0.6 MB of disk while Anvil runs.
- Stopping a run stops the whole process group of each process it started,
  including the machine servers of the rollups node and what the harness
  leaves running. Ctrl-C, SIGTERM, and SIGHUP stop a run cleanly and still
  write its report.
- `make clean-interop` removes all of `_interop/`, including the captured
  cases (a full `--scenarios all` capture takes about an hour). It drops no
  database. `./daveinterop clean` keeps the captured cases (unless `--cases`)
  and drops the `interop_*` databases, except those with open connections.
