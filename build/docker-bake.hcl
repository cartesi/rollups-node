# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

target "docker-metadata-action" {}
target "docker-platforms" {}

group "default" {
  targets = [
    "rollups-node",
  ]
}

target "rollups-node" {
  inherits   = ["docker-metadata-action", "docker-platforms"]
  dockerfile = "./build/Dockerfile"
  target     = "rollups-node"
  context    = ".."
}

target "machine" {
  dockerfile = "./machine.Dockerfile"
  target     = "server-stage"
  context    = "."
}

target "devnet" {
  dockerfile = "./devnet.Dockerfile"
  target     = "devnet"
  context    = "./devnet"
}
