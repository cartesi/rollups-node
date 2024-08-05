# Authority Claimer

This service submits rollups claims consumed from the broker to the blockchain using the [tx-manager crate](https://github.com/cartesi/tx-manager).
It runs at the end of every epoch, when new claims are inserted on the broker.

### Multidapp Mode

(This is an **experimental** feature! Don't try it unless you know exactly what you are doing!)

The `authority-claimer` can be configured to run in "multidapp mode".
To do so, the `DAPP_CONTRACT_ADDRESS` environment variable must be left unset.
This will force the claimer to instantiate a `MultidappBrokerListener` instead of a `DefaultBrokerListener`.

In multidapp mode, the claimer reads claims from the broker for multiple dapps.
All dapps must share the same History contract and the same chain ID.

Instead of using evironment variables,
    the claimer will get the list of dapp addresses from Redis,
    through the `experimental-dapp-addresses-config` key.
You must set this key with a string of comma separated (`", "`)
    hex encoded addresses (without `"0x"`)
    **before** starting the claimer.
You may rewrite this key at any time, and the claimer will adjust accordingly to the new list of addresses.
The claimer stops with an error if the list is empty.

Example key value: `"0202020202020202020202020202020202020202, 0505050505050505050505050505050505050505"`.
