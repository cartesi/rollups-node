# State Fold

[![Build Status](https://github.com/cartesi-corp/state-fold/actions/workflows/rust.yml/badge.svg)](https://github.com/cartesi-corp/state-fold/actions)

At Cartesi, we run into issues related to lack of mature tooling everyday.
Blockchain is a nascent technology, and the software stack is still in its early days and evolving at a fast pace.
Writing _ad-hoc_ solutions to these issues is not scalable.
A better approach is building abstractions on top of established lower-level solutions, in a way that can be reused.

It must be noted that these lower-level solutions are open source, developed by the community.
In the same way we've benefited from those, we are publishing our own in-house tools that may benefit the community.
The issue we are going to tackle here is reading the state of the blockchain.
This is the first step of interacting with the blockchain: to decide what we'll do at each moment, we first need to know the state of our smart contracts.

## Reading the State of the Blockchain

The state of the blockchain is ever-moving.
With each new block, things may arbitrarily change.
The goal of a _reader_ is to keep up with these changes.
Note that readers are dApp specific, and must be created to fit specific smart contracts.

The canonical way of interacting with the blockchain is through Ethereum's JSON-RPC API.
Usually the API is wrapped in some high-level library like `web3.js`.
Trust in the remote server that implements this API is imperative.
A malicious server could give us wrong responses, or simply not answer us.
This server must communicate with the Ethereum network and, as such, must run an Ethereum node.

There are generally two ways of setting this server up.
The first one is doing it ourselves.
We choose an implementation of the Ethereum protocol, such as Geth or Parity, and run it in some machine.
This setup is not trivial.
The second way is delegating this to some external provider, such as Infura or Alchemy.
This is a much easier solution, but has associated costs.
These services charge for each query past a certain limit.
Readers that decide to use Infura must take this into consideration.

One convenient way smart contracts publish their state is through _diff_ events.
The idea is simple: at each state change, we notify only what was changed.
For example, imagine a smart contract that stores all past prices of a certain asset.
Instead of emitting an event with all past prices every time a new entry is added, we emit an event with only the new entry.
This way, the reader can reconstruct the entire list by accumulating all past events.
Complex data structures like graphs and collections can only be efficiently communicated through diffs.
We run into those quite frequently at Cartesi.

The main obstacle for creating a consistent reader has to do with consensus finality.
Finality is related to persistence: once a block is committed, it may not be altered.
However, things in Ethereum are not exactly written in stone.
At any point, history may be rewritten through a process called a _chain reorganization_, and some state we read in the past may no longer be part of the blockchain.

Chain reorganizations usually happen when two or more blocks are mined at the same time, creating alternate realities.
At some point, one of the realities will be considered the correct one and the others will die off.
If we read the state of one that died off, we may run into inconsistencies.
For instance, imagine we're dealing with diff events.
If we mix up events from different realities, we end up in an inconsistent state.

The silver lining is that Ethereum has what's called _probabilistic finality_.
The chance of reorganizations become vanishingly small the further we go into the past.
As such, one simple solution to creating a consistent reader is waiting for blocks to become old.
We can calibrate the age of the blocks to some safety threshold: the older it is, the safer the reader.
However, this results in a high-latency reader.

Therefore, creating both a reader that is consistent and real-time is not trivial.
The issue becomes even worse when we consider that each dApp has its own specificities like events and getters.
We also want a reader that avoids making excessive calls to the remote server, since we want to use services like Infura.
Our reader should be able to run locally as well as remotely, and generally be lightweight and fast.

## The State Fold

The State Fold is our solution to this issue of reading blockchain state.
Its design is heavily inspired by ideas from functional programming, which allows us to create a more robust software.
The underlying insight is that the state of the blockchain is immutable _at a specific block hash_.
The latest block is ever changing, and blocks at a certain number may change due to reorganizations.
However, blocks with a specific hash never change.
They may eventually be revoked from the blockchain, but they remain immutable.
And with this immutability we can drag in concepts from functional programming.

In functional programming, _fold_ (sometimes called _reduce_) is commonly used to iterate over collections when we want to accumulate its elements.
It is a higher-order function, which takes an initial value and combining operator, and yields a final value.
If you're not familiar with this concept, we recommend learning more about it! Here is [Wikipedia's article](https://en.wikipedia.org/wiki/Fold_(higher-order_function)) on this topic, which should be a good introduction.

The first issue we have to deal with is flexibility.
Since readers are dApp specific, there must be a way of configuring the State Fold to adapt to each smart contract's needs.
The State Fold accomplishes this through programmability: the developer specifies its behavior through a callback object that acts as a delegate.
These delegates are dApp dependent, and implement the necessary blockchain state fetching logic required for each specific smart contract.

To this end, the developer must provide a pair of functions: sync and fold.
Both sync and fold create a _block state_, corresponding to the state of a smart contract at a specific block.
Sync creates a new state from scratch by querying the blockchain.
Fold advances the state by one step, using both the previous state and blockchain queries.
Note that these queries can only be done at a specific block hash, making them immutable.

At a high-level, the State Fold logic is reasonably easy.
The State Fold calls sync to produce the first state, and then uses fold to update the state one block at a time.
As such, when we first turn on the State Fold, it will call sync to generate the initial state.
Then, at every new block, it will call fold on the previous state, generating the next state.
The property these two functions must satisfy is that syncing to a block and folding up to that block has to yield the same result.
We can think of syncing at a block as a fast-track of folding from genesis to this block.

Going into more details, the branching nature of the blockchain yields a tree-like structure of blocks.
For example, if multiple miners create a block at the same time, the blockchain will temporarily fork, creating separate realities.
The State Fold stores these realities internally in a tree-like data-structure, representing the blockchain itself.
This structure associates blocks to _block states_, which fully describes to the state of our smart contracts at each block in the blockchain.
This structure is simply a cache, since we may reconstruct it entirely from the blockchain.

When a new block is added to the blockchain, the State Fold finds the parent of this new block in the cache and advances its state one step through folding.
If there are gaps in the cache (for example, it only has the grandparent block), this procedure is applied recursively.
If the cache does not contain an ancestor block, we use sync instead.
This process elegantly keeps consistency when faced with reorganizations, which enables the State Fold to operate at real-time without fear of inconsistent states.

Inside the delegate, the developer has the entire JSON-RPC reader methods available, albeit restricted to queries only at certain block hashes (because of our immutability restriction).
This allows for the same flexibility as using raw `web3.js`, but real-time, with consistency guarantees and without added complexity.

Our implementation is in Rust, available as a library (in Rust parlance, a crate).
The delegates are also written in Rust, and must be compiled with the library.
As such, it can run locally as well as remotely.

## Analysis

A good tool that solves the same problem in a different way is called _The Graph_.
However, The Graph made certain design choices with an unpalatable trade-off profile for certain parts of our software stack.
The first thing to note is that our goals are a lot more modest than The Graph, which in turn has made The Graph a lot more complex than necessary for our needs.
Here we make a short analysis of what goals our reader achieves, to justify our choice of rolling out our own solution.

The first goal is flexibility.
Flexibility is the capacity in which the reader is capable of reading arbitrary observable blockchain states.
We consider a state observable if we can read it through the Ethereum JSON-RPC API.
Our solution exposes the entire API to the developer, which makes it maximally flexible.

Programming our State Fold is done entirely in Rust.
We provide an actual strongly-typed programming language, in which the developer actively fetches data using the standard API.
We believe this imposes fewer restrictions, simplifies the overall computational model and reduces cognitive load.

Note that this flexibility, in turn, enables multiple optimizations to make a lighter use of services like Infura.
These services, past a certain limit, charge users for each query.
The State Fold, if programmed correctly, is very light on them.
Also, when folding, we naturally reuse past states, bypassing the need for querying Infura too much.

It should be noted that the State Fold works on any provider.
All it needs is an Ethereum JSON-RPC endpoint.
We designed our reader to be lightweight and to be easy on the provider.
One consequence of this is that there's not much overhead when syncing.
It can also be easily deployed either locally or remotely on our own infrastructure, which simplifies using it.

The second goal is real-time consistency.
Consistency is the degree in which the observed state is correct in relation to the blockchain, and real-time is the ability to do so at the latest block.
The challenge is creating a reader that can operate at the "quantum foam" while safely dealing with reorganizations.
We achieve these properties through the use of the functional programming fold.

Another design goal that should not be overlooked is the ability to compose State Folds.
This allows for a modular approach when writing delegates, which greatly simplifies implementing complex logic and enables reuse of code.
The ability of creating abstractions is one of the most important tools in a programmer's toolbox.

## Examples

There are a few examples in the `examples` directory that can be a good start for the new comers.

* delegate_example
* delegate_client_example
* delegate_server_example

You can choose also choose upon which Solidity data type your example will run on.

* Array
* Struct
* Mapping

### Prerequisites

* solc 0.7.5+
* geth 1.9.24+

### Run delegate example

While running this example, you can select which data type you want to use by utilising the `-m` command line argument.

The following command will run the delegate example using the Mapping data type:

```bash
cd examples
./run_delegate_example.sh -m Mapping
```

The program output would look like this
```log
Data after modify: {1: 20}
Data after modify: {2: 12, 1: 20}
Data after delete {2: 12}
Data after modify: {2: 12, 3: 9}
Data after modify: {2: 12, 3: 9, 4: 14}
Data after delete {3: 9, 4: 14}
Data after modify: {5: 97, 3: 9, 4: 14}
Current block: 9
ContractCtx { mapping: {5: 97, 3: 9, 4: 14} }
Current block: 10
ContractCtx { mapping: {5: 97, 3: 9, 4: 14} }
Current block: 11
ContractCtx { mapping: {5: 97, 3: 9, 4: 14} }
Current block: 12
ContractCtx { mapping: {5: 97, 3: 9, 4: 14} }
```

### Run delegate server and client examples:

While running these examples, you can also select which data type you want to use by utilising the `-m` command line argument.

The following command will run the delegate server example using the Mapping data type:

```bash
cd examples
./run_delegate_server_example.sh -m Mapping
```
Server will start listening for requests
```log
StateFoldServer listening on [::1]:50051
```

Open another terminal to run client
```bash
cd examples
./run_delegate_client_example.sh
```
Client should receive state from the server
```log
RESPONSE=GetStateResponse { json_state: "state: ContractState { ctx: ContractCtx { mapping: {3: 9, 5: 97, 4: 14} } }" }
```

It's worth noting that if no data type is specified, the default is `Array`.
