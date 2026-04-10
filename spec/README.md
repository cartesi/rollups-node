# Formal Specifications

This directory contains TLA+ formal specifications for critical subsystems of the Cartesi Rollups Node. TLA+ is a mathematical language for specifying and verifying concurrent and distributed systems, developed by Leslie Lamport at Microsoft Research.

## Why TLA+?

The rollups node coordinates multiple services through a shared database with advisory event notifications. Failures (lost notifications, crashed services, stale connections) can interact in subtle ways. TLA+ lets us:

- **Model** the system at a high level (state machines, not Go code)
- **Exhaustively check** all possible interleavings of concurrent actions
- **Verify** that safety properties (no data corruption) and liveness properties (work eventually gets done) hold under arbitrary failure patterns

A TLA+ spec is not a substitute for tests — it operates at a different level. Tests verify that code does what you wrote. TLA+ verifies that what you wrote is correct.

## Directory Structure

```
spec/
    README.md               # This file
    events/                 # Hybrid event system (LISTEN/NOTIFY + polling)
        HybridEvents.tla    # Main specification module
        HybridEvents.cfg    # TLC model-checking configuration
        MC.tla              # Model-checking wrapper with concrete constants
    drain-protocol/         # Application drain protocol (safe removal)
        DrainProtocol.tla   # Main specification module
        DrainProtocol.cfg   # TLC model-checking configuration
        MC.tla              # Model-checking wrapper with concrete constants
```

Each module follows the same pattern: `spec/<module-name>/`.

## Installation

### Option A: TLA+ Tools (Command Line)

The TLA+ tools (TLC model checker, TLAPS proof system, parser) are distributed as Java JAR files.

**Prerequisites**: Java 11+ (`java -version` to check).

```bash
# Download the latest tla2tools.jar
mkdir -p ~/tla
curl -L -o ~/tla/tla2tools.jar \
  https://github.com/tlaplus/tlaplus/releases/latest/download/tla2tools.jar
```

Verify:

```bash
java -jar ~/tla/tla2tools.jar -help
```

### Option B: TLA+ Toolbox (GUI)

The TLA+ Toolbox is an Eclipse-based IDE with integrated model checking.

1. Download from https://github.com/tlaplus/tlaplus/releases
2. Pick the archive for your OS (macOS, Linux, Windows)
3. Extract and run

### Option C: VS Code Extension

1. Install the **TLA+** extension (`alygin.vscode-tlaplus`) from the VS Code marketplace
2. The extension bundles the TLA+ tools — no separate install needed
3. Commands available via the Command Palette (`Cmd+Shift+P`):
   - `TLA+: Parse module` — check syntax
   - `TLA+: Check model with TLC` — run the model checker
   - `TLA+: Evaluate expression` — evaluate a TLA+ expression

## Running the Model Checker

### From the command line

```bash
cd spec/events

# Parse only (syntax check) — uses the SANY parser
# Note: use -cp (classpath), not -jar, to invoke SANY directly
java -cp ~/tla/tla2tools.jar tla2sany.SANY HybridEvents.tla

# Run TLC model checker
java -jar ~/tla/tla2tools.jar \
  -config HybridEvents.cfg \
  -workers auto \
  MC.tla
```

The `-workers auto` flag uses all available CPU cores. On a modern laptop, the events model (~10^5 states) completes in under 60 seconds.

### From VS Code

1. Open `MC.tla`
2. `Cmd+Shift+P` -> `TLA+: Check model with TLC`
3. Output appears in the TLA+ Output panel

### From the TLA+ Toolbox

1. Open the `.tla` file
2. Create a new model (`TLC Model Checker` -> `New Model`)
3. Set constants, invariants, and temporal properties as specified in the `.cfg` file
4. Click `Run TLC`

## Reading TLA+ Specifications

If you are new to TLA+, start here:

1. **Lamport's video course** (free): https://lamport.azurewebsites.net/video/videos.html — 10 short lectures covering the core language
2. **"Specifying Systems"** (free PDF): https://lamport.azurewebsites.net/tla/book.html — the definitive reference
3. **Learn TLA+** (community guide): https://learntla.com — practical, example-driven introduction

### Quick syntax reference

| TLA+ | Meaning |
|------|---------|
| `x' = expr` | Next-state value of `x` (primed variables are the "after" state) |
| `UNCHANGED <<x, y>>` | `x' = x /\ y' = y` (these variables do not change) |
| `/\` | Logical AND |
| `\/` | Logical OR |
| `=>` | Implies |
| `\in` | Element of |
| `\E x \in S : P(x)` | There exists an `x` in `S` such that `P(x)` |
| `\A x \in S : P(x)` | For all `x` in `S`, `P(x)` holds |
| `[]P` | `P` is always true (safety / invariant) |
| `<>P` | `P` is eventually true (liveness) |
| `[]<>P` | `P` is true infinitely often |
| `WF_vars(A)` | Weak fairness: if action `A` is continuously enabled, it eventually happens |

### How to read a spec

1. Start with the **comment block** at the top — it describes what the module models
2. Read **VARIABLES** — these are the state of the system
3. Read **Init** — the initial state
4. Read **Next** — one step of the system (a disjunction of all possible actions)
5. Read **Invariants** — safety properties that must always hold
6. Read **Temporal properties** — liveness properties (things that must eventually happen)
7. Read `MC.tla` — the concrete constants used for model checking

## Interpreting TLC Output

**Success** looks like:

```
Model checking completed. No error has been found.
  Estimates of the probability that TLC did not check all reachable states
  because two distinct states had the same fingerprint:
  ...
```

**Failure** (invariant violation) looks like:

```
Error: Invariant Safety_NoDuplicateProcessing is violated.
Error: The behavior up to this point is:
State 1: ...
State 2: ...
...
```

The trace shows the exact sequence of states that leads to the violation. This is the counterexample — it tells you exactly how the system can break.

## Adding a New Specification

1. Create a new directory: `spec/<module-name>/`
2. Create three files:
   - `<ModuleName>.tla` — the specification
   - `<ModuleName>.cfg` — TLC configuration (constants, invariants, properties)
   - `MC.tla` — model-checking wrapper with concrete constant values
3. Document the module in this README under "Directory Structure"
4. Verify with `java -jar ~/tla/tla2tools.jar -config <ModuleName>.cfg -workers auto MC.tla`
