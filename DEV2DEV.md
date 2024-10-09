# Dev 2 Dev

```bash
git checkout feature/new-build
git submodule update --init --recursive

# to fix
# fatal: No url found for submodule path 'offchain/grpc-interfaces/grpc-interfaces' in .gitmodules
# run:
git rm --cached offchain/grpc-interfaces/grpc-interfaces

# download machine emulator header files
git submodule add https://github.com/cartesi/machine-emulator-sdk.git

cd machine-emulator-sdk
git checkout v0.19.0
# output
# HEAD is now at afaade1 feat!: Bump solidity-step to v0.12.1

make toolchain

sudo apt update
sudo apt install libslirp-dev
sudo apt install liblua5.4-dev
# sudo apt install libboost-all-dev
# or
cd emulator
make bundle-boost

make emulator
```
Running `sudo make install` I got this error:

```bash
chmod 0755 /usr/bin/cartesi-machine /usr/bin/cartesi-machine-stored-hash
cp -RP tools/gdb /usr/share/cartesi-machine/gdb
make[1]: Leaving directory '/workspace/rollups-node/machine-emulator-sdk/emulator'
cd kernel/artifacts && install -p -m 0644 linux-6.5.13-ctsi-1-v0.20.0.bin /usr/share/cartesi-machine/images
install: cannot stat 'linux-6.5.13-ctsi-1-v0.20.0.bin': No such file or directory
make: *** [Makefile:122: install] Error 1
```

Trying to fix this error I run

```bash
make kernel
```

```bash
make toolchain
make create-symlinks
```

install the https://github.com/cartesi/genext2fs/releases/tag/v1.5.6

```bash
docker buildx create --name mybuilder --use --bootstrap --platform linux/amd64,linux/amd64/v2,linux/amd64/v3,linux/386,linux/riscv64
make tools
```

```bash
cd ..
make generate
```


```bash
make build
```

Update the tools/fs/Dockerfile with this content:

```Dockerfile
FROM --platform=$BUILDPLATFORM ubuntu:22.04 AS cross-builder
ENV BUILD_BASE=/tmp/build-extra

# Install dependencies
RUN DEBIAN_FRONTEND=noninteractive apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    wget \
    patch \
    libdigest-sha-perl \
    libc6-dev-riscv64-cross \
    gcc-12-riscv64-linux-gnu \
    && \
    adduser developer -u 499 --gecos ",,," --disabled-password && \
    mkdir -p ${BUILD_BASE} && chown -R developer:developer ${BUILD_BASE} && \
    rm -rf /var/lib/apt/lists/*

USER developer
WORKDIR ${BUILD_BASE}

# Build benchmark binaries
COPY fs/dhrystone.patch ${BUILD_BASE}/
COPY fs/shasumfile ${BUILD_BASE}/
RUN mkdir benchmarks && cd benchmarks && \
    wget https://www.netlib.org/benchmark/whetstone.c https://www.netlib.org/benchmark/dhry-c && \
    shasum -ca 256 ../shasumfile &&\
    bash dhry-c && \
    patch -p1 < ../dhrystone.patch && \
    riscv64-linux-gnu-gcc-12 -O2 -o whetstone whetstone.c -lm && \
    riscv64-linux-gnu-gcc-12 -O2 -o dhrystone dhry_1.c dhry_2.c -lm

# Final image
FROM --platform=linux/riscv64 riscv64/ubuntu:22.04
ARG TOOLS_DEB=machine-emulator-tools-v0.15.0.deb
ADD ${TOOLS_DEB} /tmp/
RUN apt-get update
RUN apt-get install -y curl xxd --allow-downgrades
RUN apt-get install -y --no-install-recommends --allow-downgrades \
    busybox-static=1:1.30.1-7ubuntu3 \
    coreutils=8.32-4.1ubuntu1 \
    bash=5.1-6ubuntu1 \
    psmisc=23.4-2build3 \
    bc=1.07.1-3build1 \
    device-tree-compiler=1.6.1-1 \
    jq=1.6-2.1ubuntu3 \
    lua5.4=5.4.4-1 \
    lua-socket=3.0~rc1+git+ac3201d-6 \
    file=1:5.41-3ubuntu0.1 \
    /tmp/${TOOLS_DEB} \
    && \
    rm -rf /var/lib/apt/lists/* /tmp/${TOOLS_DEB}
COPY --chown=root:root --from=cross-builder /tmp/build-extra/benchmarks/whetstone /usr/bin/
COPY --chown=root:root --from=cross-builder /tmp/build-extra/benchmarks/dhrystone /usr/bin/
```

## Marcelo's instructions

### Build the Machine Emulator

```bash
cd machine-emulator-sdk/emulator

# choose a folder to install
make install PREFIX=/YOUR-WORKSPACE/rollups-node/emulator-install
```

Example:

```bash
make install PREFIX=/Users/oshiro/calindra/cartesi/rollups-node/emulator-install
```

### Build the node

```bash
cd /YOUR-WORKSPACE/rollups-node
PREFIX=/YOUR-WORKSPACE/rollups-node/emulator-install make 
```

Example:

```bash
PREFIX=/Users/oshiro/calindra/cartesi/rollups-node/emulator-install make 
```

### Build the anvil devnet

```bash
make devnet
```

### Run the evm-reader

Configure the environment

```bash
eval `make env`
```

Run the db and anvil:

```bash
make run-postgres
make run-devnet
```

Restore the database:

```bash
./cartesi-rollups-cli db upgrade -p "postgres://postgres:password@localhost:5432/rollupsdb?sslmode=disable"
```

Run the evm-advancer:

```bash
./cartesi-rollups-evm-reader
```
