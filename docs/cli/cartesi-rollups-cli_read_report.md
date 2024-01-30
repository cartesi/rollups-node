## cartesi-rollups-cli read report

Reads a report

```
cartesi-rollups-cli read report [flags]
```

### Examples

```
# Read report 5 from input 6:
cartesi-rollups-cli read report --report-index 5 --input-index 6
```

### Options

```
      --graphql-endpoint string   address used to connect to graphql (default "http://localhost:10000/graphql")
  -h, --help                      help for report
      --input-index int           index of the input
      --report-index int          index of the report
```

### SEE ALSO

* [cartesi-rollups-cli read](cartesi-rollups-cli_read.md)	 - Read the node state from the GraphQL API

