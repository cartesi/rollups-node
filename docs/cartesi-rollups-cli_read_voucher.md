## cartesi-rollups-cli read voucher

Reads a voucher

```
cartesi-rollups-cli read voucher [flags]
```

### Examples

```
# Read voucher 5 from input 6:
cartesi-rollups-cli read voucher --voucher-index 5 --input-index 6
```

### Options

```
      --graphql-endpoint string   address used to connect to graphql (default "http://localhost:10000/graphql")
  -h, --help                      help for voucher
      --input-index int           index of the input
      --voucher-index int         index of the voucher
```

### SEE ALSO

* [cartesi-rollups-cli read](cartesi-rollups-cli_read.md)	 - Read the node state from the GraphQL API

