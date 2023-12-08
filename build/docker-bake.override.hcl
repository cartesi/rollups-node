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

target "machine-snapshot" {
  tags = ["${DOCKER_ORGANIZATION}/rollups-machine-snapshot:${TAG}"]
}

target "devnet" {
  tags = ["${DOCKER_ORGANIZATION}/rollups-devnet:${TAG}"]
}
