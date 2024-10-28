// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

// A Service implementation that does nothing
type DummyService struct {
	Service
}

type CreateDummyInfo struct {
	CreateInfo
}

func CreateDummy(ci CreateDummyInfo, null *DummyService) error {
	return Create(&ci.CreateInfo, &null.Service)
}

func (s *DummyService) Alive() bool       { return true }
func (s *DummyService) Ready() bool       { return true }
func (s *DummyService) Reload() []error   { return nil }
func (s *DummyService) Tick() []error     { return nil }
func (s *DummyService) Stop(bool) []error { return nil }
