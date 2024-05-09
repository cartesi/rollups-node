# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

variable "TAG" {
  default = "devel"
}

variable "DOCKER_ORGANIZATION" {
  default = "cartesi"
}

target "rollups-node" {
  tags = ["${DOCKER_ORGANIZATION}/rollups-node:${TAG}"]
}

target "rollups-node-snapshot" {
  tags = ["${DOCKER_ORGANIZATION}/rollups-node-snapshot:${TAG}"]
}

target "rollups-node-devnet" {
  tags = ["${DOCKER_ORGANIZATION}/rollups-node-devnet:${TAG}"]
}

target "rollups-node-ci-base" {
  tags = ["${DOCKER_ORGANIZATION}/rollups-node-ci-base:${TAG}"]
}
