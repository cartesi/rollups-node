// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/ethclient"
)

func newDelivery(
	_ context.Context, c *CreateInfo, s *Service, _ *PersistentConfig, chainID *big.Int,
) (*delivery, error) {
	if c.AdapterFactory != nil {
		s.adapterFactory = c.AdapterFactory
	} else {
		client, ok := s.client.(*ethclient.Client)
		if !ok {
			return nil, fmt.Errorf("EthClient must be *ethclient.Client when AdapterFactory is not provided")
		}
		s.adapterFactory = NewDefaultAdapterFactory(client, s.filter)
	}
	policy := txsender.RuntimePolicy()
	var err error
	if s.submissionEnabled {
		policy, err = deliveryPolicy(c.Config)
		if err != nil {
			return nil, fmt.Errorf("PRT transaction policy: %w", err)
		}
	}
	d := &delivery{
		Service: s, deliveryPolicy: policy,
		deliverySweeps: map[actionKind]*applicationSweep{}, candidateCooldown: map[actionID]time.Time{},
	}
	d.rootBondRecoveryScans = make(map[int64]*rootBondRecoveryScan)
	if !s.submissionEnabled {
		return d, nil
	}
	backend := c.DeliveryBackend
	if backend == nil {
		client, ok := s.client.(*ethclient.Client)
		if !ok {
			return nil, fmt.Errorf("PRT submission needs a concrete Ethereum client or explicit DeliveryBackend")
		}
		backend = txsender.ProtectedClient{Client: client}
	}
	d.deliveryBackend = backend
	d.sender, err = txsender.New[actionID](backend, s.txOptsFactory, chainID.Uint64(), policy, d.guardPublication, s.Logger)
	if err != nil {
		return nil, fmt.Errorf("PRT transaction sender: %w", err)
	}
	return d, nil
}
