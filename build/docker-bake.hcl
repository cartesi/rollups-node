# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

target "docker-metadata-action" {}
target "docker-platforms" {}

group "default" {
  targets = [
    "rollups-node",
    "rollups-node-snapshot",
    "rollups-node-devnet",
    "rollups-node-cli",
  ]
}

target "common" {
  inherits   = ["docker-platforms", "docker-metadata-action"]
  dockerfile = "./build/Dockerfile"
  context    = ".."
  args       = {
    BASE_IMAGE                   = "debian:bookworm-20240311-slim"
    RUST_VERSION                 = "1.78.0"
    GO_VERSION                   = "1.22.1"
    FOUNDRY_NIGHTLY_VERSION      = "626221f5ef44b4af950a08e09bd714650d9eb77d"
    MACHINE_EMULATOR_VERSION     = "0.18.1"
    MACHINE_TOOLS_VERSION        = "0.16.1"
    MACHINE_IMAGE_KERNEL_VERSION = "0.20.0"
    MACHINE_KERNEL_VERSION       = "6.5.13"
    MACHINE_XGENEXT2FS_VERSION   = "1.5.6"
  }
}

target "rollups-node" {
  inherits = ["common"]
  target   = "rollups-node"
  args     = {
    ROLLUPS_NODE_VERSION = "devel"
  }
}

target "rollups-node-cli" {
  inherits = ["common"]
  target   = "rollups-node-cli"
  args     = {
    ROLLUPS_NODE_VERSION = "devel"
  }
}

target "rollups-node-snapshot" {
  inherits = ["common"]
  target   = "rollups-node-snapshot"
}

target "rollups-node-devnet" {
  inherits = ["common"]
  target   = "rollups-node-devnet"
}

target "rollups-node-ci" {
  inherits   = ["common"]
  target     = "rollups-node-ci"
  dockerfile = "./Dockerfile"
  context    = "."
}
