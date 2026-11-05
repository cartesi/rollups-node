// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package inspectclient contains the generated OpenAPI inspection client.
package inspectclient

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -generate types,client -o generated.go -package inspectclient ../../api/openapi/inspect.yaml

//go:generate go run ./internal/jsonv2gen generated.go
