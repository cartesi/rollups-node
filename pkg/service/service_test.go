// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

func (s *ServeSuite) TestBaseTemplate_InitializesLoggerAndReady() {
	svcName := "test service"

	svc := &BaseTemplate{}
	InitServiceTemplate(svc, &BaseConfigs{Name: svcName})

	s.NotNil(svc.Logger)
	s.Equal(svc.String(), svcName)
	s.True(svc.Ready())
}
