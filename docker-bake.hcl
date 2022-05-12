
target "docker-metadata-action" {}

group "default" {
  targets = ["delegate_server", "dispatcher", "indexer", "reader"]
}

target "delegate_server" {
  inherits   = ["docker-metadata-action"]
  dockerfile = "offchain/delegate_server/Dockerfile"
  context    = "."
}

target "dispatcher" {
  inherits   = ["docker-metadata-action"]
  dockerfile = "offchain/offchain/Dockerfile"
  context    = "."
}

target "indexer" {
  inherits   = ["docker-metadata-action"]
  dockerfile = "offchain/indexer/Dockerfile"
  context    = "."
}

target "reader" {
  inherits   = ["docker-metadata-action"]
  dockerfile = "Dockerfile"
  context    = "./reader"
}
