## cartesi-rollups-cli inspect

Calls inspect API

```
cartesi-rollups-cli inspect [flags]
```

### Examples

```
# Makes a request with "hi" encoded as hex:
cartesi-rollups-cli inspect --payload 0x$(printf "hi" | xxd -p)
```

### Options

```
  -h, --help                      help for inspect
      --inspect-endpoint string   address used to connect to the inspect api (default "http://localhost:10000/")
      --payload string            input payload hex-encoded starting with 0x
```

### SEE ALSO

* [cartesi-rollups-cli](cartesi-rollups-cli.md)	 - Command line interface for Cartesi Rollups

