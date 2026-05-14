// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"log/slog"

	"github.com/ethereum/go-ethereum/common"

	. "github.com/cartesi/rollups-node/internal/model"
)

// cachedAdapters stores contract adapters along with the configuration fields
// used to create them, enabling staleness detection when app config changes.
type cachedAdapters struct {
	applicationContract ApplicationContractAdapter
	inputSource         InputSourceAdapter
	daveConsensus       DaveConsensusAdapter
	consensusAddr       common.Address
	inputBoxAddr        common.Address
	isDaveConsensus     bool
	hasInputBoxDA       bool
}

type applicationAdapterResolver struct {
	logger  *slog.Logger
	factory AdapterFactory
	cache   map[common.Address]cachedAdapters
}

func newApplicationAdapterResolver(
	logger *slog.Logger,
	factory AdapterFactory,
) *applicationAdapterResolver {
	return &applicationAdapterResolver{
		logger:  logger,
		factory: factory,
		cache:   map[common.Address]cachedAdapters{},
	}
}

func (r *applicationAdapterResolver) buildAppContracts(apps []*Application) []appContracts {
	r.evictRemovedApplications(apps)

	contracts := make([]appContracts, 0, len(apps))
	for _, app := range apps {
		cached, ok := r.getOrCreateAdapters(app)
		if !ok {
			continue
		}
		contracts = append(contracts, appContracts{
			application:         app,
			applicationContract: cached.applicationContract,
			inputSource:         cached.inputSource,
			daveConsensus:       cached.daveConsensus,
		})
	}
	return contracts
}

func (r *applicationAdapterResolver) evictRemovedApplications(apps []*Application) {
	activeAddrs := make(map[common.Address]struct{}, len(apps))
	for _, app := range apps {
		activeAddrs[app.IApplicationAddress] = struct{}{}
	}
	for addr := range r.cache {
		if _, active := activeAddrs[addr]; active {
			continue
		}
		r.logger.Debug("Evicting cached adapters for removed application", "address", addr)
		delete(r.cache, addr)
	}
}

func (r *applicationAdapterResolver) getOrCreateAdapters(app *Application) (cachedAdapters, bool) {
	addr := app.IApplicationAddress
	cached, cacheHit := r.cache[addr]
	if cacheHit && adaptersAreStale(cached, app) {
		r.logger.Info(
			"Application contract configuration changed, recreating adapters",
			"application", app.Name,
			"address", addr,
		)
		delete(r.cache, addr)
		cacheHit = false
	}
	if cacheHit {
		return cached, true
	}

	appContract, inputSource, daveConsensus, err := r.factory.CreateAdapters(app)
	if err != nil {
		r.logger.Error("Error retrieving application contracts", "app", app, "error", err)
		return cachedAdapters{}, false
	}
	cached = cachedAdapters{
		applicationContract: appContract,
		inputSource:         inputSource,
		daveConsensus:       daveConsensus,
		consensusAddr:       app.IConsensusAddress,
		inputBoxAddr:        app.IInputBoxAddress,
		isDaveConsensus:     app.IsDaveConsensus(),
		hasInputBoxDA:       app.HasDataAvailabilitySelector(DataAvailability_InputBox),
	}
	r.cache[addr] = cached
	return cached, true
}

func adaptersAreStale(cached cachedAdapters, app *Application) bool {
	return cached.consensusAddr != app.IConsensusAddress ||
		cached.inputBoxAddr != app.IInputBoxAddress ||
		cached.isDaveConsensus != app.IsDaveConsensus() ||
		cached.hasInputBoxDA != app.HasDataAvailabilitySelector(DataAvailability_InputBox)
}
