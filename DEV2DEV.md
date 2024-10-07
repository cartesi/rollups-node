```
git checkout feature/new-build
git submodule update --init --recursive

# download machine emulator header files
git submodule add https://github.com/cartesi/machine-emulator-sdk.git
git checkout v0.19.0
cd machine-emulator-sdk
make toolchain

make build
```