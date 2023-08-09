
variable "TAG" {
  default = "devel"
}

variable "DOCKER_ORGANIZATION" {
  default = "cartesi"
}

target "state-server" {
  tags = ["${DOCKER_ORGANIZATION}/rollups-state-server:${TAG}"]
}

target "dispatcher" {
  tags = ["${DOCKER_ORGANIZATION}/rollups-dispatcher:${TAG}"]
}

target "indexer" {
  tags = ["${DOCKER_ORGANIZATION}/rollups-indexer:${TAG}"]
}

target "inspect-server" {
  tags = ["${DOCKER_ORGANIZATION}/rollups-inspect-server:${TAG}"]
}

target "advance-runner" {
  tags = ["${DOCKER_ORGANIZATION}/rollups-advance-runner:${TAG}"]
}

target "graphql-server" {
  tags = ["${DOCKER_ORGANIZATION}/rollups-graphql-server:${TAG}"]
}

target "host-runner" {
  tags = ["${DOCKER_ORGANIZATION}/rollups-host-runner:${TAG}"]
}
