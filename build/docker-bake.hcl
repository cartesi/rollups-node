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

target "rollups-node" {
  inherits   = ["docker-metadata-action", "docker-platforms"]
  dockerfile = "./build/Dockerfile"
  target     = "rollups-node"
  context    = ".."
}

target "rollups-node-snapshot" {
  inherits    = ["docker-platforms"]
  dockerfile  = "./build/Dockerfile"
  target      = "rollups-node-snapshot"
  context     = ".."
}

target "rollups-node-devnet" {
  inherits    = ["docker-platforms"]
  dockerfile  = "./build/Dockerfile"
  target      = "rollups-node-devnet"
  context     = ".."
}
