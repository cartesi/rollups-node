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
    BASE_IMAGE                = "debian:bookworm-20240130-slim"
    RUST_VERSION              = "1.76.0"
    GO_VERSION                = "1.22.0"
    FOUNDRY_NIGHTLY_VERSION   = "293fad73670b7b59ca901c7f2105bf7a29165a90"
    SERVER_MANAGER_VERSION    = "0.8.3"
    MACHINE_EMULATOR_VERSION  = "0.15.3"
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
