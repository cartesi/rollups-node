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
