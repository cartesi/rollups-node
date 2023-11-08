# Authority Claimer

This service submits rollups claims consumed from the broker to the blockchain using the [tx-manager crate](https://github.com/cartesi/tx-manager).
It runs at the end of every epoch, when new claims are inserted on the broker.
