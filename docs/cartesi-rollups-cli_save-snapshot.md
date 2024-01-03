## cartesi-rollups-cli save-snapshot

Saves the testing Cartesi machine snapshot to the designated folder

```
cartesi-rollups-cli save-snapshot [flags]
```

### Examples

```
# Save the default Rollups Echo Application snapshot:
cartesi-rollups-cli save-snapshot
```

### Options

```
      --dest-dir string              directory where to store the Cartesi Machine snapshot to be used by the local Node (default "./machine-snapshot")
      --docker-image string          Docker image containing the Cartesi Machine snapshot to be used (default "cartesi/rollups-machine-snapshot:devel")
  -h, --help                         help for save-snapshot
      --temp-container-name string   Name of the temporary container needed to extract the machine snapshot files (default "temp-machine")
```

### SEE ALSO

* [cartesi-rollups-cli](cartesi-rollups-cli.md)	 - Command line interface for Cartesi Rollups

