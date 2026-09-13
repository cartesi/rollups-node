// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package ethutil

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/pkg/contracts/iapplicationfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveappfactory"
)

func TestPRTDeploymentSendsFactoryArguments(t *testing.T) {
	withdrawalConfig := iapplicationfactory.WithdrawalConfig{
		Guardian: common.HexToAddress("0x1000000000000000000000000000000000000001"),
	}

	for _, test := range []struct {
		name     string
		manager  common.Address
		sentries []common.Address
	}{
		{name: "no sentries"},
		{name: "manager without slots", manager: common.HexToAddress("0x10")},
		{name: "fixed slots", sentries: []common.Address{common.HexToAddress("0x30"), common.HexToAddress("0x20")}},
		{name: "rotatable slots", manager: common.HexToAddress("0x10"),
			sentries: []common.Address{common.HexToAddress("0x30"), common.HexToAddress("0x20")}},
	} {
		for _, claimStagingPeriod := range []uint64{0, 1234} {
			t.Run(fmt.Sprintf("%s/period_%d", test.name, claimStagingPeriod), func(t *testing.T) {
				parsed, err := idaveappfactory.IDaveAppFactoryMetaData.GetAbi()
				require.NoError(t, err)
				predicted := common.HexToAddress("0x400")
				response, err := parsed.Methods["calculateDaveAppAddress"].Outputs.Pack(predicted, common.HexToAddress("0x500"))
				require.NoError(t, err)
				backend := &prtFactoryCallBackend{calls: make(chan prtFactoryCall, 1), response: response}
				server := rpc.NewServer()
				require.NoError(t, server.RegisterName("eth", backend))
				defer server.Stop()
				client := ethclient.NewClient(rpc.DialInProc(server))
				defer client.Close()
				deployment := &PRTApplicationDeployment{
					SentryManager: test.manager, Sentries: test.sentries, ApplicationDeployment: ApplicationDeployment{
						FactoryAddress:     common.HexToAddress("0x200"),
						TemplateHash:       common.HexToHash("0x300"),
						Salt:               SaltBytes{31: 4},
						ClaimStagingPeriod: claimStagingPeriod,
						WithdrawalConfig:   withdrawalConfig,
					},
				}

				opts := &bind.TransactOpts{
					Nonce: big.NewInt(0), GasPrice: big.NewInt(1), GasLimit: 1_000_000, NoSend: true,
					Signer: func(_ common.Address, tx *types.Transaction) (*types.Transaction, error) { return tx, nil },
				}
				var deployData []byte
				address, result, err := deployment.DeployWithTransaction(t.Context(), client, NewStaticTransactOptsFactory(opts),
					func(
						_ context.Context, opts *bind.TransactOpts, build func(*bind.TransactOpts) (*types.Transaction, error),
					) (*types.Receipt, error) {
						tx, err := build(opts)
						require.NoError(t, err)
						require.Equal(t, deployment.FactoryAddress, *tx.To())
						deployData = tx.Data()
						return nil, nil
					})
				require.NoError(t, err)
				require.Equal(t, predicted, address)
				require.Nil(t, result)
				call := <-backend.calls
				require.Equal(t, deployment.FactoryAddress, call.To)
				for methodName, data := range map[string][]byte{"calculateDaveAppAddress": call.Input, "newDaveApp": deployData} {
					method := parsed.Methods[methodName]
					require.Equal(t, method.ID, data[:4])
					args, err := method.Inputs.Unpack(data[4:])
					require.NoError(t, err)
					require.Equal(t, [32]byte(deployment.TemplateHash), args[0])
					require.Zero(t, new(big.Int).SetUint64(claimStagingPeriod).Cmp(args[1].(*big.Int)))
					require.Equal(t, test.manager, args[2])
					require.Equal(t, append([]common.Address{}, test.sentries...), args[3])
					gotWithdrawal := abi.ConvertType(args[4], new(idaveappfactory.WithdrawalConfig)).(*idaveappfactory.WithdrawalConfig)
					require.Equal(t, idaveappfactory.WithdrawalConfig(withdrawalConfig), *gotWithdrawal)
					require.Equal(t, [32]byte(deployment.Salt), args[5])
				}
			})
		}
	}
}

type prtFactoryCall struct {
	To    common.Address `json:"to"`
	Input hexutil.Bytes  `json:"input"`
}

type prtFactoryCallBackend struct {
	calls    chan prtFactoryCall
	response hexutil.Bytes
}

func (b *prtFactoryCallBackend) Call(_ context.Context, call prtFactoryCall, _ string) (hexutil.Bytes, error) {
	b.calls <- call
	return b.response, nil
}

func (*prtFactoryCallBackend) GetCode(context.Context, common.Address, string) (hexutil.Bytes, error) {
	return hexutil.Bytes{}, nil
}

func TestPRTDeploymentRejectsInvalidSentriesBeforeRPC(t *testing.T) {
	address := common.HexToAddress("0x01")
	for _, test := range []struct {
		name      string
		sentries  []common.Address
		wantError string
	}{
		{name: "zero", sentries: []common.Address{{}}, wantError: "must not be the zero address"},
		{name: "duplicate", sentries: []common.Address{address, address}, wantError: "duplicates address"},
	} {
		t.Run(test.name, func(t *testing.T) {
			deployment := &PRTApplicationDeployment{Sentries: test.sentries}
			_, _, err := deployment.Deploy(t.Context(), nil, nil)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}
