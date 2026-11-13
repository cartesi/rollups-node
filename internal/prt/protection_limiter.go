// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/prtprotection"
	"github.com/ethereum/go-ethereum/common"
)

type protectionNoticeKind uint8

const (
	protectionRootStop protectionNoticeKind = iota
	protectionBacklog
	protectionClockDiscovery
	protectionBoundaryDiscovery
	protectionForeclosureWait
	protectionForeclosureRefusal
)

type protectionNoticeKey struct {
	root common.Address
	kind protectionNoticeKind
}

type protectionNoticeIdentity struct {
	cause                         string
	generation                    int64
	epoch, close                  uint64
	available, closed, lastChance bool
}

type protectionNotice struct {
	identity protectionNoticeIdentity
	warnedAt time.Time
}

func (s *Service) protectionNoticeDue(appID int64, key protectionNoticeKey, identity protectionNoticeIdentity) bool {
	if s.protectionNotices == nil {
		s.protectionNotices = make(map[int64]map[protectionNoticeKey]protectionNotice)
	}
	if s.protectionNotices[appID] == nil {
		s.protectionNotices[appID] = make(map[protectionNoticeKey]protectionNotice)
	}
	previous := s.protectionNotices[appID][key]
	if previous.identity == identity && !warningDue(previous.warnedAt) {
		return false
	}
	s.protectionNotices[appID][key] = protectionNotice{identity: identity, warnedAt: time.Now()}
	return true
}

// Each discovery site owns its incident. Success at another site cannot clear it.
func (s *Service) reportProtectionDiscovery(app *model.Application, kind protectionNoticeKind, cause error) {
	key := protectionNoticeKey{kind: kind}
	if cause == nil {
		delete(s.protectionNotices[app.ID], key)
		return
	}
	// Moving heads and provider wrapper text do not identify a new incident.
	if s.protectionNoticeDue(app.ID, key, protectionNoticeIdentity{}) {
		prtprotection.ReportUnavailable(s.Logger, app, cause)
	}
}

// Root completion is independent of local finalization and sibling exits.
// Reuse the observer notice limiter; a moving head is not a new incident.
func (s *Service) reportForeclosureRootWait(app *model.Application, epoch *model.Epoch, root *model.Tournament, head uint64) {
	standing := "unobserved"
	var asOf uint64
	if root != nil {
		standing, asOf = root.Snapshot.Standing.String(), root.Snapshot.AsOfBlock
	}
	key := protectionNoticeKey{root: *epoch.TournamentAddress, kind: protectionForeclosureWait}
	if !s.protectionNoticeDue(app.ID, key, protectionNoticeIdentity{epoch: epoch.Index, cause: standing}) {
		return
	}
	s.Logger.Info("Foreclosed PRT epoch awaits a complete terminal root observation; no runtime is required for this wait",
		"application", app.Name, "epoch_index", epoch.Index, "tournament", epoch.TournamentAddress,
		"standing", standing, "as_of_block", asOf, "observed_head", head, "foreclose_block", app.ForecloseBlock)
}

// Claim-transaction epochs can need comparison without an unjoined clock.
// The drain page owns their notice lifetime, independently of clock discovery.
func (s *Service) pruneForeclosureWaitNotices(appID int64, epochs []*model.Epoch) {
	for key := range s.protectionNotices[appID] {
		if !isForeclosureNotice(key.kind) {
			continue
		}
		present := false
		for _, epoch := range epochs {
			if epoch.Index == s.protectionNotices[appID][key].identity.epoch &&
				(epoch.TournamentAddress != nil && *epoch.TournamentAddress == key.root ||
					epoch.TournamentAddress == nil && key.root == (common.Address{})) {
				present = true
				break
			}
		}
		if !present {
			delete(s.protectionNotices[appID], key)
		}
	}
}

func isForeclosureNotice(kind protectionNoticeKind) bool {
	return kind == protectionForeclosureWait || kind == protectionForeclosureRefusal
}

func (s *Service) clearForeclosureEpochNotices(appID int64, epoch uint64) {
	for key, notice := range s.protectionNotices[appID] {
		if isForeclosureNotice(key.kind) && notice.identity.epoch == epoch {
			delete(s.protectionNotices[appID], key)
		}
	}
}

func (s *Service) clearForeclosureNotices(appID int64) {
	for key := range s.protectionNotices[appID] {
		if isForeclosureNotice(key.kind) {
			delete(s.protectionNotices[appID], key)
		}
	}
}

func foreclosureNoticeRoot(epoch *model.Epoch) common.Address {
	if epoch.TournamentAddress != nil {
		return *epoch.TournamentAddress
	}
	return common.Address{}
}

func (s *Service) reportForeclosureRefusal(app *model.Application, epoch *model.Epoch, head uint64, generation int64) {
	root := foreclosureNoticeRoot(epoch)
	waitKey := protectionNoticeKey{root: root, kind: protectionForeclosureWait}
	if notice, present := s.protectionNotices[app.ID][waitKey]; present && notice.identity.epoch == epoch.Index {
		return
	}
	key := protectionNoticeKey{root: root, kind: protectionForeclosureRefusal}
	if !s.protectionNoticeDue(app.ID, key, protectionNoticeIdentity{epoch: epoch.Index, generation: generation}) {
		return
	}
	s.Logger.Info("PRT foreclosure write deferred; waiting for updated eligibility or evidence",
		"application", app.Name, "epoch_index", epoch.Index, "observed_head", head,
		"foreclose_block", app.ForecloseBlock, "reset_generation", generation)
}
