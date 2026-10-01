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
| `check --for capture\|replay\|suite\|run-sling` | Reports the prerequisites of one capability. Installs nothing. |
| `capture` | Runs a Dave harness scenario and records its chain as a case directory. |
| `replay` | Loads a case into a test-owned Anvil, attaches a fresh rollups node, and verifies it. `--then` continues on the same chain. |
| `verify` | Compares a running rollups node with the chain and the Sling node's evidence. |
| `run-sling` | Attaches the Sling node to an existing chain in the foreground, for manual sessions. |

`make daveinterop` builds `./daveinterop`; the `make interop-*` targets build
it first and pass `ARGS`:

| Make target | Command |
| --- | --- |
| `make interop-suite ARGS="..."` | `suite` (builds the node first) |
| `make interop-check ARGS="--for replay"` | `check` |
| `make interop-capture ARGS="..."` | `capture` |
| `make interop-replay ARGS="--case DIR"` | `replay` (builds the node first) |
| `make interop-verify ARGS="--app NAME ..."` | `verify` |
| `make interop-sling ARGS="..."` | `run-sling` |

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
| `epochs.sealed` | Every sealed epoch has the same bounds, last block, and tournament, and a computed commitment; every other node epoch is the open one, with the right bounds |
| `epochs.staged` | Final state, outputs root, and block equal `EpochStaged`; every epoch the node calls staged is staged on chain |
| `epochs.accepted` | Every accepted epoch is `CLAIM_ACCEPTED` with the accepting transaction, and the reverse |
| `commitments.sling` | Commitments equal the Sling node's stage claims and joins. The Sling node logs its commitment only when it stages, and a commitment is joined only once, so an epoch that the rollups node joined first has no Sling commitment; the detail lists those epochs as not compared |
| `final_states.sling` | Final states equal the Sling node's joins and sentry claims |
| `commitments.root_winner` | Every finished root has the node's commitment as winner (divergence check); roots still running are listed |
| `tournaments` | Every tournament, with its parent and epoch |
| `tournaments.commitments` | Every `CommitmentJoined` event, with final state, submitter, block, and transaction |
| `tournaments.matches` | Every `MatchCreated` event, with its commitments, block, and transaction, and every `MatchDeleted` |
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
