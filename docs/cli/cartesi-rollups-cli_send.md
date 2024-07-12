## cartesi-rollups-cli send

Send a rollups input to the Ethereum node

```
cartesi-rollups-cli send [flags]
```

### Examples

```
# Send the string "hi" encoded as hex:
cartesi-rollups-cli send --payload 0x$(printf "hi" | xxd -p)
```

### Options

```
      --account uint32        account index used to sign the transaction (default: 0)
      --address-book string   if set, load the address book from the given file; else, use test addresses
      --eth-endpoint string   ethereum node JSON-RPC endpoint (default "http://localhost:8545")
  -h, --help                  help for send
      --mnemonic string       mnemonic used to sign the transaction (default "test test test test test test test test test test test junk")
      --payload string        input payload hex-encoded starting with 0x
      --verbose               If set, prints all debug logs
```

### SEE ALSO

* [cartesi-rollups-cli](cartesi-rollups-cli.md)	 - Command line interface for Cartesi Rollups

