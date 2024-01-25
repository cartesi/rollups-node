## cartesi-rollups-cli read notice

Reads a notice

```
cartesi-rollups-cli read notice [flags]
```

### Examples

```
# Read notice 5 from input 6:
cartesi-rollups-cli read notice --notice-index 5 --input-index 6
```

### Options

```
      --graphql-endpoint string   address used to connect to graphql (default "http://localhost:10000/graphql")
  -h, --help                      help for notice
      --input-index int           index of the input
      --notice-index int          index of the notice
```

### SEE ALSO

* [cartesi-rollups-cli read](cartesi-rollups-cli_read.md)	 - Read the node state from the GraphQL API

