## cartesi-rollups-cli mine

Mine blocks

```
cartesi-rollups-cli mine [flags]
```

### Examples

```
# Mine 10 blocks with a 5-second interval between them:
cartesi-rollups-cli mine --number-of-blocks 10 --block-interval 5
```

### Options

```
      --block-interval int     interval, in seconds, between the timestamps of each block (default 1)
      --eth-endpoint string    ethereum node JSON-RPC endpoint (default "http://localhost:8545")
  -h, --help                   help for mine
      --number-of-blocks int   number of blocks to mine (default 1)
```

### SEE ALSO

* [cartesi-rollups-cli](cartesi-rollups-cli.md)	 - Command line interface for Cartesi Rollups

