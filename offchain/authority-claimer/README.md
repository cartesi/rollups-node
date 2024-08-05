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
This key holds a Redis Set value.
You must use commands such as SADD and SREM to manipulate the list of addresses.
Addresses are encoded as hex strings without the leading `"0x"`.
Redis values are case sensitive, so addresses must be in lowercase format.
Example address value: `"0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"`.

You may rewrite the list of addresses at any time,
    the claimer will adjust accordingly.
The list of addresses can be empty at any time,
    the claimer will wait until an application address is added to the set to resume operations.
