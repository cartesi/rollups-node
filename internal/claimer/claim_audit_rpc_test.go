// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

type claimAuditRPC struct {
	receipt *types.Receipt
	header  *types.Header
}

func (r *claimAuditRPC) GetTransactionReceipt(context.Context, common.Hash) (any, error) {
	if r.receipt == nil {
		return nil, nil
	}
	return r.receipt, nil
}

func (r *claimAuditRPC) GetBlockByNumber(context.Context, rpc.BlockNumber, bool) (*types.Header, error) {
	return r.header, nil
}

func TestClaimAuditReceiptRequiresCanonicalSuccessfulInclusionAtSelectedView(t *testing.T) {
	for _, scenario := range []string{"canonical", "missing", "orphaned", "newer than selected", "failed", "wrong hash"} {
		t.Run(scenario, func(t *testing.T) {
			header := deliveryHeader(44)
			hash := common.HexToHash("0xcafe")
			api := &claimAuditRPC{header: header, receipt: &types.Receipt{
				Status: types.ReceiptStatusSuccessful, TxHash: hash, BlockNumber: big.NewInt(44), BlockHash: header.Hash(),
				Logs: []*types.Log{},
			}}
			selected := uint64(60)
			var expectedError error
			switch scenario {
			case "missing":
				api.receipt = nil
			case "orphaned":
				api.receipt.BlockHash = common.HexToHash("0xbad")
			case "newer than selected":
				selected = 43
				expectedError = txsender.ErrInconclusive
			case "failed":
				api.receipt.Status = types.ReceiptStatusFailed
			case "wrong hash":
				api.receipt.TxHash = common.HexToHash("0xbad")
				expectedError = txsender.ErrInconclusive
			}
			server := rpc.NewServer()
			require.NoError(t, server.RegisterName("eth", api))
			t.Cleanup(server.Stop)
			client := ethclient.NewClient(rpc.DialInProc(server))
			t.Cleanup(client.Close)
			chain := &claimerBlockchain{client: client, backend: txsender.ProtectedClient{Client: client}}
			canonical, err := chain.claimTransactionCanonical(t.Context(), hash, selected)
			if expectedError != nil {
				require.ErrorIs(t, err, expectedError)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, scenario == "canonical", canonical)
		})
	}
}
