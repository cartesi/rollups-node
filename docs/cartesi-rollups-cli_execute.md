## cartesi-rollups-cli execute

Executes a voucher

```
cartesi-rollups-cli execute [flags]
```

### Examples

```
# Executes voucher 5 from input 6:
cartesi-rollups-cli execute --voucher-index 5 --input-index 6
```

### Options

```
      --account uint32            account index used to sign the transaction (default: 0)
      --address-book string       if set, load the address book from the given file; else, use test addresses
      --eth-endpoint string       ethereum node JSON-RPC endpoint (default "http://localhost:8545")
      --graphql-endpoint string   address used to connect to graphql (default "http://0.0.0.0:10004/graphql")
  -h, --help                      help for execute
      --input-index int           index of the input
      --mnemonic string           mnemonic used to sign the transaction (default "test test test test test test test test test test test junk")
      --voucher-index int         index of the voucher
```

### SEE ALSO

* [cartesi-rollups-cli](cartesi-rollups-cli.md)	 - Command line interface for Cartesi Rollups

