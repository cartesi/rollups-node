
target "docker-metadata-action" {}
target "docker-platforms" {}

group "default" {
  targets = [
    "advance-runner",
    "dispatcher",
    "graphql-server",
    "host-runner",
    "inspect-server",
    "indexer",
    "state-server"
  ]
}

target "deps" {
  inherits   = ["docker-metadata-action", "docker-platforms"]
  dockerfile = "offchain/Dockerfile"
  target     = "builder"
  context    = "."
}

target "state-server" {
  inherits   = ["docker-metadata-action", "docker-platforms"]
  dockerfile = "offchain/Dockerfile"
  target     = "state_server"
  context    = "."
}

target "dispatcher" {
  inherits   = ["docker-metadata-action", "docker-platforms"]
  dockerfile = "offchain/Dockerfile"
  target     = "dispatcher"
  context    = "."
}

target "indexer" {
  inherits   = ["docker-metadata-action", "docker-platforms"]
  dockerfile = "offchain/Dockerfile"
  target     = "indexer"
  context    = "."
}

target "inspect-server" {
  inherits   = ["docker-metadata-action", "docker-platforms"]
  dockerfile = "offchain/Dockerfile"
  target     = "inspect_server"
  context    = "."
}

target "graphql-server" {
  inherits   = ["docker-metadata-action", "docker-platforms"]
  dockerfile = "offchain/Dockerfile"
  target     = "graphql_server"
  context    = "."
}

target "advance-runner" {
  inherits   = ["docker-metadata-action", "docker-platforms"]
  dockerfile = "offchain/Dockerfile"
  target     = "advance_runner"
  context    = "."
}

target "host-runner" {
  inherits   = ["docker-metadata-action", "docker-platforms"]
  dockerfile = "offchain/Dockerfile"
  target     = "host_runner"
  context    = "."
}
