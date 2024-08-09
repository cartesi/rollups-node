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
    BASE_IMAGE                   = "debian:bookworm-20240311-slim"
    RUST_VERSION                 = "1.78.0"
    GO_VERSION                   = "1.22.1"
    FOUNDRY_NIGHTLY_VERSION      = "293fad73670b7b59ca901c7f2105bf7a29165a90"
    MACHINE_EMULATOR_VERSION     = "0.17.0"
    MACHINE_TOOLS_VERSION        = "0.15.0"
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
