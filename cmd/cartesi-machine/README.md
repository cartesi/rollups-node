# Go bindings to the Cartesi Machine C API

## Quick Start

Ensure that the emulator headers and libraries are installed or point to them with:
```
export CGO_CFLAGS="-I/foo/machine-emulator/src"
export CGO_LDFLAGS="-L/foo/machine-emulator/src"

```

Build
```
go build
``` 

Point to the directory containing the image files
```
export CARTESI_IMAGES_PATH=<path-to-image-files>
```

Run
```
go run cmd/cartesi-machine/main.go --help
go run cmd/cartesi-machine/main.go
go run cmd/cartesi-machine/main.go --command="ls -l"
go run cmd/cartesi-machine/main.go --max-mcycle=0 --store=/tmp/maquina
go run cmd/cartesi-machine/main.go --load=/tmp/maquina --command="ls -l"
go run cmd/cartesi-machine/main.go --load=/tmp/maquina --initial-hash --final-hash
go run cmd/cartesi-machine/main.go --remote-address="localhost:5000"--load=/tmp/maquina --initial-hash --final-hash --command="ls -l"
``` 