## cartesi-rollups-cli run-deps

Run node dependencies with Docker

```
cartesi-rollups-cli run-deps [flags]
```

### Examples

```
# Run all deps:
cartesi-rollups-cli run-deps
```

### Options

```
      --devnet-docker-image string     Devnet docker image name (default "cartesi/rollups-node-devnet:devel")
      --devnet-mapped-port string      devnet local listening port number (default "8545")
  -h, --help                           help for run-deps
      --postgres-docker-image string   Postgress docker image name (default "postgres:16-alpine")
      --postgres-mapped-port string    Postgres local listening port number (default "5432")
      --postgres-password string       Postgres password (default "password")
```

### SEE ALSO

* [cartesi-rollups-cli](cartesi-rollups-cli.md)	 - Command line interface for Cartesi Rollups

