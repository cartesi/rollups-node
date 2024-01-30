## cartesi-rollups-cli validate

Validates a notice

```
cartesi-rollups-cli validate [flags]
```

### Examples

```
# Validates notice 5 from input 6:
cartesi-rollups-cli validate --notice-index 5 --input-index 6
```

### Options

```
      --address-book string       if set, load the address book from the given file; else, use test addresses
      --eth-endpoint string       ethereum node JSON-RPC endpoint (default "http://localhost:8545")
      --graphql-endpoint string   address used to connect to graphql (default "http://localhost:10000/graphql")
  -h, --help                      help for validate
      --input-index int           index of the input
      --notice-index int          index of the notice
```

### SEE ALSO

* [cartesi-rollups-cli](cartesi-rollups-cli.md)	 - Command line interface for Cartesi Rollups

