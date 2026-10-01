// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"fmt"

	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/jsonrpc/client"
)

// apiPageSize is the default page size of the node API. The API caps each
// page; it does not cap the total number of rows.
const apiPageSize = 50

type nodeAPI struct {
	url    string
	client *client.Client
}

func newNodeAPI(url string) *nodeAPI {
	return &nodeAPI{url: url, client: client.NewClient(url)}
}

func (a *nodeAPI) application(ctx context.Context, app string) (*model.Application, error) {
	var response api.SingleResponse[model.Application]
	if err := a.client.Call(ctx, "cartesi_getApplication", api.GetApplicationParams{Application: app}, &response); err != nil {
		return nil, fmt.Errorf("cartesi_getApplication: %w", err)
	}
	return &response.Data, nil
}

// listAll reads every page of a list method. It rejects an empty page before
// the reported total, a changing total, and duplicate pages.
func listAll[T any](ctx context.Context, a *nodeAPI, method string, params func(limit, offset uint64) any) ([]T, error) {
	var all []T
	var total uint64
	for offset := uint64(0); ; {
		var page api.ListResponse[T]
		if err := a.client.Call(ctx, method, params(apiPageSize, offset), &page); err != nil {
			return nil, fmt.Errorf("%s (offset %d): %w", method, offset, err)
		}
		if offset == 0 {
			total = page.Pagination.TotalCount
		} else if page.Pagination.TotalCount != total {
			return nil, fmt.Errorf("%s: total changed from %d to %d during pagination", method, total, page.Pagination.TotalCount)
		}
		all = append(all, page.Data...)
		offset += uint64(len(page.Data))
		if offset >= total {
			return all, nil
		}
		if len(page.Data) == 0 {
			return nil, fmt.Errorf("%s: empty page at offset %d of %d", method, offset, total)
		}
	}
}

func (a *nodeAPI) epochs(ctx context.Context, app string) ([]model.Epoch, error) {
	return listAll[model.Epoch](ctx, a, "cartesi_listEpochs", func(limit, offset uint64) any {
		return api.ListEpochsParams{Application: app, Limit: limit, Offset: offset}
	})
}

func (a *nodeAPI) inputs(ctx context.Context, app string) ([]model.Input, error) {
	return listAll[model.Input](ctx, a, "cartesi_listInputs", func(limit, offset uint64) any {
		return api.ListInputsParams{Application: app, Limit: limit, Offset: offset}
	})
}

func (a *nodeAPI) inputsByTransaction(ctx context.Context, app, txHash string) ([]model.Input, error) {
	return listAll[model.Input](ctx, a, "cartesi_listInputs", func(limit, offset uint64) any {
		return api.ListInputsParams{Application: app, TransactionHash: &txHash, Limit: limit, Offset: offset}
	})
}

func (a *nodeAPI) outputs(ctx context.Context, app string) ([]model.Output, error) {
	return listAll[model.Output](ctx, a, "cartesi_listOutputs", func(limit, offset uint64) any {
		return api.ListOutputsParams{Application: app, Limit: limit, Offset: offset}
	})
}

func (a *nodeAPI) commitments(ctx context.Context, app string) ([]model.Commitment, error) {
	return listAll[model.Commitment](ctx, a, "cartesi_listCommitments", func(limit, offset uint64) any {
		return api.ListCommitmentsParams{Application: app, Limit: limit, Offset: offset}
	})
}

func (a *nodeAPI) matches(ctx context.Context, app string) ([]model.Match, error) {
	return listAll[model.Match](ctx, a, "cartesi_listMatches", func(limit, offset uint64) any {
		return api.ListMatchesParams{Application: app, Limit: limit, Offset: offset}
	})
}

func (a *nodeAPI) withdrawals(ctx context.Context, app string) ([]model.Withdrawal, error) {
	return listAll[model.Withdrawal](ctx, a, "cartesi_listWithdrawals", func(limit, offset uint64) any {
		return api.ListWithdrawalsParams{Application: app, Limit: limit, Offset: offset}
	})
}

func (a *nodeAPI) tournaments(ctx context.Context, app string) ([]model.Tournament, error) {
	return listAll[model.Tournament](ctx, a, "cartesi_listTournaments", func(limit, offset uint64) any {
		return api.ListTournamentsParams{Application: app, Limit: limit, Offset: offset}
	})
}
