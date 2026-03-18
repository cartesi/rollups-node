// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

// ClaimerSuite contains repository test cases for claimer-related queries.
// TODO: add test cases.
type ClaimerSuite struct {
	BaseSuite
}

func NewClaimerSuite(factory RepositoryFactory) *ClaimerSuite {
	return &ClaimerSuite{BaseSuite: BaseSuite{factory: factory}}
}
