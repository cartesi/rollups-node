// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This package contains auxiliary functions generated with genqlient
// to query the Rollups GraphQL API
package inspectclient

//go:generate go run github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen -generate types,client -o generated.go -package inspectclient ../../api/openapi/inspect.yaml
