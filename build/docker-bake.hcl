# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

target "docker-metadata-action" {}
target "docker-platforms" {}

group "default" {
  targets = [
    "rollups-node", "machine-snapshot", "devnet"
  ]
}

target "rollups-node" {
  inherits   = ["docker-metadata-action", "docker-platforms"]
  dockerfile = "./build/Dockerfile"
  target     = "rollups-node"
  context    = ".."
}

target "machine-snapshot" {
  inherits    = ["docker-platforms"]
  dockerfile  = "./build/Dockerfile"
  target      = "machine-snapshot"
  context     = ".."
}

target "devnet" {
  inherits    = ["docker-platforms"]
  dockerfile  = "./build/Dockerfile"
  target      = "devnet"
  context     = ".."
}
