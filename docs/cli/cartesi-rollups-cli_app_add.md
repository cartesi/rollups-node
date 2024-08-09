## cartesi-rollups-cli app add

Adds a new application

```
cartesi-rollups-cli app add [flags]
```

### Examples

```
# Adds an application to Rollups Node:
cartesi-rollups-cli app add -a 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF -i 0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
```

### Options

```
  -a, --address string               Application contract address
  -h, --help                         help for add
  -i, --iconsensus string            Application IConsensus Address
  -n, --inputbox-block-number uint   InputBox deployment block number
  -u, --snapshot-uri string          Application snapshot URI
  -s, --status string                Sets the application status (default "running")
  -t, --template-hash string         Application template hash
```

### Options inherited from parent commands

```
  -p, --postgres-endpoint string   Postgres endpoint (default "postgres://postgres:password@localhost:5432/postgres")
  -v, --verbose                    verbose output
```

### SEE ALSO

* [cartesi-rollups-cli app](cartesi-rollups-cli_app.md)	 - Application management related commands

