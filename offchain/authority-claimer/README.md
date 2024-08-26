# Authority Claimer

This service submits rollups claims consumed from the broker to the blockchain using the [tx-manager crate](https://github.com/cartesi/tx-manager).
It runs at the end of every epoch, when new claims are inserted on the broker.

### Multi-dapp Mode

(This is an **experimental** feature! Don't try it unless you know exactly what you are doing!)

The `authority-claimer` can be configured to run in "multidapp mode".
To do so, the `DAPP_CONTRACT_ADDRESS` environment variable must be left unset.
This will force the claimer to instantiate a `MultidappBrokerListener` instead of a `DefaultBrokerListener`.

In multidapp mode, the claimer reads claims from the broker for multiple applications.
All dapps must share the same History contract and the same chain ID.

Instead of using evironment variables,
    the claimer will get the list of application addresses from Redis,
    through the `experimental-dapp-addresses-config` key.
This key holds a [Redis Set](https://redis.io/docs/latest/develop/data-types/sets/) value.
You must use commands such as [`SADD`](https://redis.io/docs/latest/commands/sadd/) and [`SREM`]https://redis.io/docs/latest/commands/srem/() to manipulate the set of addresses.

Application addresses must be encoded as hex strings.
The prefix `0x` is ignored by the `authority-claimer` when loading applications adresses.
However, it's advised to use it as a matter of convention.
Example address value: `"0x0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"`.

> [!NOTE]
> Duplicate addresses as well as malformed addresses are detected and logged.

> [!TIP]
> Application addresses are case insensitive even though Redis values are not.
> So, even though `"0x00000000000000000000000000000000deadbeef"` and `"0x00000000000000000000000000000000DeadBeef"` may belong to the same Redis Set, they are considered to be the same application address and one of them will be identified as a duplicate.

You may update the contents of `experimental-dapp-addresses-config` at any time,
    the claimer will adjust accordingly.
The set of addresses may be emptied at any time as well,
    the claimer will wait until an application address is added to the set before resuming operations.
