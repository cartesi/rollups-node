package template

import (
	"github.com/cartesi/rollups-node/pkg/service"
)

type CreateInfo struct {
	service.CreateInfo
}

type Service struct {
	service.Service
}

type Metrics struct {
}

func Create(ci CreateInfo, s *Service) error {
	if err := service.Create(ci.CreateInfo, s, &s.Service); err != nil {
		return err
	}
	return nil
}

func (s *Service) Alive() bool {
	return true
}

func (s *Service) Ready() bool {
	return true
}

func (s *Service) Reload() bool {
	return true
}

func (s *Service) Tick() bool {
	return true
}

