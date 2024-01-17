# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

target "docker-metadata-action" {}
target "docker-platforms" {}

group "default" {
  targets = [
    "rollups-node",
    "rollups-node-snapshot",
    "rollups-node-devnet",
  ]
}

target "common" {
  inherits   = ["docker-platforms", "docker-metadata-action"]
  dockerfile = "./build/Dockerfile"
  context    = ".."
  args       = {
    BASE_IMAGE                = "debian:bookworm-20240110-slim"
    RUST_VERSION              = "1.75.0"
    GO_VERSION                = "1.21.1"
    FOUNDRY_COMMIT_VERSION    = "24abca6c9133618e0c355842d2be2dd4f36da46d"
    ROLLUPS_CONTRACTS_VERSION = "1.1.0"
    SERVER_MANAGER_VERSION    = "0.8.2"
    MACHINE_EMULATOR_VERSION  = "0.15.2"
    ROOTFS_VERSION            = "0.18.0"
    LINUX_VERSION             = "0.17.0"
    LINUX_KERNEL_VERSION      = "5.15.63-ctsi-2-v0.17.0"
    ROM_VERSION               = "0.17.0"
  }
}

target "rollups-node" {
  inherits = ["common"]
  target   = "rollups-node"
}

target "rollups-node-snapshot" {
  inherits = ["common"]
  target   = "rollups-node-snapshot"
}

target "rollups-node-devnet" {
  inherits = ["common"]
  target   = "rollups-node-devnet"
}
