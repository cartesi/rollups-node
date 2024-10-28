// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import "time"

// A Service implementation that takes time to Tick
type SlowService struct {
	Service
	Time time.Duration
}

type CreateSlowInfo struct {
	CreateInfo
	Time time.Duration
}

func CreateSlow(ci CreateSlowInfo, slow *SlowService) error {
	slow.Time = ci.Time
	return Create(&ci.CreateInfo, &slow.Service)
}

func (me *SlowService) Alive() bool {
	return true
}

func (me *SlowService) Ready() bool {
	return true
}

func (me *SlowService) Reload() []error {
	return nil
}

func (me *SlowService) Tick() []error {
	time.Sleep(me.Time)
	return nil
}

func (me *SlowService) Stop(bool) []error {
	return nil
}
