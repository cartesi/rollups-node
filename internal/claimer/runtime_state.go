// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

func (s *Service) hasClaimInFlight(appID int64) bool {
	_, ok := s.claimsInFlight[appID]
	return ok
}

func (s *Service) putClaimInFlight(appID int64, tx inFlightTx) {
	s.claimsInFlight[appID] = tx
}

func (s *Service) dropClaimInFlight(appID int64) {
	delete(s.claimsInFlight, appID)
}

func (s *Service) hasAcceptInFlight(appID int64) bool {
	_, ok := s.acceptsInFlight[appID]
	return ok
}

func (s *Service) putAcceptInFlight(appID int64, tx inFlightTx) {
	s.acceptsInFlight[appID] = tx
}

func (s *Service) dropAcceptInFlight(appID int64) {
	delete(s.acceptsInFlight, appID)
}

func (s *Service) incrementAcceptAttempt(key acceptAttemptKey) uint64 {
	s.acceptAttempts[key]++
	return s.acceptAttempts[key]
}

func (s *Service) dropAcceptAttempt(key acceptAttemptKey) {
	delete(s.acceptAttempts, key)
}
