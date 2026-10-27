// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"testing"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

const (
	testDeploymentWordBytes  = 32
	testFactorySignature     = "getTournamentFactory()"
	testParametersSignature  = "tournamentParameters(uint64)"
	testMarchSignature       = "CM_MARCHID()"
	wantTableDeployment      = "table"
	wantGenerationDeployment = "generation"
)

var testDeploymentPristine = common.HexToHash("0x558f38f2350970a0a566b2450afa64250059c1cff66c9a55560e5b93fa783926")

type deploymentRPCReply struct {
	data []byte
	err  error
}
type deploymentRPCError struct {
	code    int
	message string
}

func (e deploymentRPCError) Error() string  { return e.message }
func (e deploymentRPCError) ErrorCode() int { return e.code }

type deploymentRPCRecord struct {
	address common.Address
	input   []byte
	block   rpc.BlockNumberOrHash
}
type deploymentViewRPC struct {
	mu                             sync.Mutex
	consensus, factory, transition common.Address
	head                           txsender.BlockRef
	rows                           [][]uint64
	count                          uint64
	march                          uint64
	pristine                       common.Hash
	absent                         map[common.Address]bool
	overrides                      map[string]deploymentRPCReply
	records                        []deploymentRPCRecord
	rowCalls                       []uint64
	codeErr                        error
}

func testDeploymentWords(values ...uint64) []byte {
	out := make([]byte, testDeploymentWordBytes*len(values))
	for i, value := range values {
		binary.BigEndian.PutUint64(out[(i+1)*testDeploymentWordBytes-8:], value)
	}
	return out
}
func testDeploymentAddress(address common.Address) []byte {
	out := make([]byte, testDeploymentWordBytes)
	copy(out[testDeploymentWordBytes-common.AddressLength:], address[:])
	return out
}
func testDeploymentSelector(signature string) []byte { return crypto.Keccak256([]byte(signature))[:4] }
func testDeploymentRequest(address common.Address, input []byte) string {
	return address.Hex() + hexutil.Encode(input)
}
func testDeploymentRowInput(level uint64) []byte {
	return append(testDeploymentSelector(testParametersSignature), testDeploymentWords(level)...)
}

func newDeploymentViewRPC(t *testing.T, rows [][]uint64, count uint64) (*DaveConsensusAdapterImpl, *deploymentViewRPC) {
	t.Helper()
	backend := &deploymentViewRPC{consensus: common.HexToAddress("0x101"), factory: common.HexToAddress("0x202"),
		transition: common.HexToAddress("0x303"), head: txsender.BlockRef{Number: 100, Hash: common.HexToHash("0x104")},
		rows: rows, count: count, march: 22, pristine: testDeploymentPristine, absent: make(map[common.Address]bool),
		overrides: make(map[string]deploymentRPCReply)}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", backend))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	adapter, err := NewDaveConsensusAdapter(backend.consensus, client)
	require.NoError(t, err)
	concrete, ok := adapter.(*DaveConsensusAdapterImpl)
	require.True(t, ok)
	return concrete, backend
}
func (b *deploymentViewRPC) record(address common.Address, input []byte, block rpc.BlockNumberOrHash) error {
	if block.BlockHash == nil || *block.BlockHash != b.head.Hash || !block.RequireCanonical || block.BlockNumber != nil {
		return errors.New("read did not use the selected canonical hash")
	}
	b.records = append(b.records, deploymentRPCRecord{address: address, input: slices.Clone(input), block: block})
	return nil
}
func (b *deploymentViewRPC) GetCode(_ context.Context, address common.Address, block rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.record(address, nil, block); err != nil {
		return nil, err
	}
	if b.codeErr != nil {
		return nil, b.codeErr
	}
	if b.absent[address] {
		return nil, nil
	}
	return hexutil.Bytes{1}, nil
}
func (b *deploymentViewRPC) Call(_ context.Context, fields map[string]json.RawMessage, block rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var address common.Address
	var input hexutil.Bytes
	if err := json.Unmarshal(fields["to"], &address); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(fields["input"], &input); err != nil {
		return nil, err
	}
	if err := b.record(address, input, block); err != nil {
		return nil, err
	}
	if b.absent[address] {
		return nil, nil
	}
	if response, ok := b.overrides[testDeploymentRequest(address, input)]; ok {
		return response.data, response.err
	}
	if address == b.consensus && slices.Equal(input, testDeploymentSelector(testFactorySignature)) {
		return testDeploymentAddress(b.factory), nil
	}
	if address == b.factory {
		switch {
		case slices.Equal(input, testDeploymentSelector("stateTransition()")):
			return testDeploymentAddress(b.transition), nil
		case slices.Equal(input, testDeploymentSelector("tournamentLevelCount()")):
			return testDeploymentWords(b.count), nil
		case len(input) == testDeploymentWordBytes+4 && slices.Equal(input[:4], testDeploymentSelector(testParametersSignature)):
			level := binary.BigEndian.Uint64(input[len(input)-8:])
			b.rowCalls = append(b.rowCalls, level)
			if level >= uint64(len(b.rows)) {
				return nil, errors.New("unexpected table row")
			}
			return testDeploymentWords(b.rows[level]...), nil
		}
	}
	if address == b.transition {
		if slices.Equal(input, testDeploymentSelector(testMarchSignature)) {
			return testDeploymentWords(b.march), nil
		}
		if slices.Equal(input, testDeploymentSelector("UARCH_PRISTINE_STATE_HASH()")) {
			return b.pristine[:], nil
		}
	}
	return nil, fmt.Errorf("unexpected deployment read at %s: %x", address, input)
}

func canonicalDeploymentRows() [][]uint64 {
	return [][]uint64{{2, 38, 54, 11, 22, 33}, {2, 0, 38, 11, 22, 33}}
}
func alternateDeploymentRows() [][]uint64 {
	return [][]uint64{{3, 44, 48, 11, 22, 33}, {3, 27, 17, 11, 22, 33}, {3, 0, 27, 11, 22, 33}}
}

func TestDeploymentReaderCompleteGeometryAndAllRowWords(t *testing.T) {
	maxValid := [][]uint64{{69, 68, 24, 11, 22, 33}}
	for stride := uint64(67); ; stride-- {
		maxValid = append(maxValid, []uint64{69, stride, 1, 11, 22, 33})
		if stride == 0 {
			break
		}
	}
	maxCount := [][]uint64{{92, 38, 54, 11, 22, 33}, {92, 0, 38, 11, 22, 33}}
	for len(maxCount) < 92 {
		maxCount = append(maxCount, []uint64{92, 0, 1, 11, 22, 33})
	}
	cases := []struct {
		name    string
		rows    [][]uint64
		count   uint64
		want    string
		fetched int
	}{
		{"canonical", canonicalDeploymentRows(), 2, "compatible", 2},
		{"canonical zero budgets", [][]uint64{{2, 38, 54, 0, 0, 0}, {2, 0, 38, 0, 0, 0}}, 2, "compatible", 2},
		{"four levels at lower bound",
			[][]uint64{
				{4, 20, 72, 11, 22, 33},
				{4, 12, 8, 11, 22, 33},
				{4, 4, 8, 11, 22, 33},
				{4, 0, 4, 11, 22, 33},
			}, 4, wantTableDeployment, 4},
		{"same count alternate", [][]uint64{{2, 44, 48, 11, 22, 33}, {2, 0, 44, 11, 22, 33}}, 2, wantTableDeployment, 2},
		{"three levels", alternateDeploymentRows(), 3, wantTableDeployment, 3},
		{"invalid final row",
			[][]uint64{{3, 44, 48, 11, 22, 33}, {3, 27, 17, 11, 22, 33}, {3, 0, 26, 11, 22, 33}}, 3, wantGenerationDeployment, 3},
		{"wrong repeated count",
			[][]uint64{{3, 44, 48, 11, 22, 33}, {3, 27, 17, 11, 22, 33}, {2, 0, 27, 11, 22, 33}}, 3, wantGenerationDeployment, 3},
		{"zero count", canonicalDeploymentRows(), 0, wantGenerationDeployment, 1},
		{"excessive count", canonicalDeploymentRows(), 93, wantGenerationDeployment, 1},
		{"one level", [][]uint64{{1, 0, 92, 11, 22, 33}}, 1, wantGenerationDeployment, 1},
		{"maximum valid", maxValid, 69, wantTableDeployment, 69},
		{"maximum calls", maxCount, 92, wantGenerationDeployment, 92},
		{"overflow root", [][]uint64{{2, math.MaxUint64, 93, 11, 22, 33}, {2, 0, 38, 11, 22, 33}}, 2, wantGenerationDeployment, 2},
		{"overflow child", [][]uint64{{2, 38, 54, 11, 22, 33}, {2, math.MaxUint64, 39, 11, 22, 33}}, 2, wantGenerationDeployment, 2},
		{"zero height",
			[][]uint64{{4, 44, 48, 11, 22, 33},
				{4, 27, 17, 11, 22, 33},
				{4, 27, 0, 11, 22, 33},
				{4, 0, 27, 11, 22, 33}}, 4, wantGenerationDeployment, 4},
		{"incomplete ruler", [][]uint64{{2, 44, 47, 11, 22, 33}, {2, 0, 44, 11, 22, 33}}, 2, wantGenerationDeployment, 2},
		{"root below bounds", [][]uint64{{2, 19, 73, 11, 22, 33}, {2, 0, 19, 11, 22, 33}}, 2, wantGenerationDeployment, 2},
		{"root above bounds", [][]uint64{{2, 69, 23, 11, 22, 33}, {2, 0, 69, 11, 22, 33}}, 2, wantGenerationDeployment, 2},
		{"nonzero leaf stride", [][]uint64{{2, 44, 48, 11, 22, 33}, {2, 1, 43, 11, 22, 33}}, 2, wantGenerationDeployment, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adapter, b := newDeploymentViewRPC(t, tc.rows, tc.count)
			evidence, err := adapter.ReadDeployment(t.Context(), b.head)
			require.NoError(t, err)
			require.Equal(t, tc.want, string(evidence.Candidate))
			require.Equal(t, b.head, evidence.Head)
			require.True(t, evidence.FactoryKnown)
			require.Equal(t, b.factory, evidence.Factory)
			require.Equal(t, b.transition, evidence.Transition)
			require.True(t, evidence.Alpha6Shape)
			require.Equal(t, tc.count, evidence.Count)
			require.Len(t, evidence.Rows, tc.fetched)
			require.Len(t, b.rowCalls, tc.fetched)
			for i, row := range evidence.Rows {
				actual := []uint64{row.Levels, row.Log2Step, row.Height, row.ResponseBudget, row.CommitmentBudget, row.MaxAllowance}
				require.Equal(t, tc.rows[i], actual)
			}
		})
	}
}

func TestDeploymentReaderRejectsOnlyConclusiveStaticFailures(t *testing.T) {
	for _, tc := range []struct {
		name, method string
		factory      bool
		data         []byte
		want         deploymentState
		shape        bool
	}{
		{"factory short", testFactorySignature, false, make([]byte, 31), deploymentGeneration, false},
		{"factory extra word", testFactorySignature, false, make([]byte, 64), deploymentGeneration, false},
		{"factory empty", testFactorySignature, false, nil, deploymentNone, false},
		{"five words", testParametersSignature, true, testDeploymentWords(2, 38, 54, 11, 22), deploymentGeneration, false},
		{"seven words", testParametersSignature, true, testDeploymentWords(2, 38, 54, 11, 22, 33, 44), deploymentGeneration, false},
		{"empty row", testParametersSignature, true, nil, deploymentNone, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, b := newDeploymentViewRPC(t, canonicalDeploymentRows(), 2)
			target := b.consensus
			input := testDeploymentSelector(tc.method)
			if tc.factory {
				target = b.factory
				input = testDeploymentRowInput(0)
			}
			b.overrides[testDeploymentRequest(target, input)] = deploymentRPCReply{data: tc.data}
			evidence, err := adapter.ReadDeployment(t.Context(), b.head)
			require.NoError(t, err)
			require.Equal(t, tc.want, evidence.Candidate)
			require.Equal(t, tc.factory, evidence.FactoryKnown)
			require.Equal(t, tc.shape, evidence.Alpha6Shape)
			require.Empty(t, evidence.Rows)
			call := evidence.Malformed
			if tc.want != deploymentGeneration {
				require.Nil(t, call)
				return
			}
			require.NotNil(t, call)
			require.Equal(t, uint64(len(tc.data)), call.ReturnLength)
			require.Equal(t, crypto.Keccak256Hash(tc.data), call.ReturnHash)
		})
	}
	t.Run("noncanonical address padding", func(t *testing.T) {
		adapter, b := newDeploymentViewRPC(t, canonicalDeploymentRows(), 2)
		raw := testDeploymentAddress(b.factory)
		raw[0] = 1
		b.overrides[testDeploymentRequest(b.consensus, testDeploymentSelector(testFactorySignature))] = deploymentRPCReply{data: raw}
		evidence, err := adapter.ReadDeployment(t.Context(), b.head)
		require.NoError(t, err)
		require.Equal(t, deploymentGeneration, evidence.Candidate)
		require.False(t, evidence.FactoryKnown)
	})
	t.Run("uint64 overflow in row", func(t *testing.T) {
		adapter, b := newDeploymentViewRPC(t, canonicalDeploymentRows(), 2)
		raw := testDeploymentWords(2, 38, 54, 11, 22, 33)
		raw[0] = 1
		b.overrides[testDeploymentRequest(b.factory, testDeploymentRowInput(0))] = deploymentRPCReply{data: raw}
		evidence, err := adapter.ReadDeployment(t.Context(), b.head)
		require.NoError(t, err)
		require.Equal(t, deploymentGeneration, evidence.Candidate)
		require.True(t, evidence.FactoryKnown)
		require.False(t, evidence.Alpha6Shape)
	})
}

func TestDeploymentReaderPartialIdentityAndUnavailableSources(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		change                      func(*deploymentViewRPC)
		want                        deploymentState
		cause                       deploymentCause
		rowCount                    int
		transition, pristine, count bool
		wantError                   bool
	}{
		{"marchid mismatch", func(b *deploymentViewRPC) { b.march = 21 },
			deploymentGeneration, deploymentCauseMarchID, 1, true, false, false, false},
		{"pristine mismatch", func(b *deploymentViewRPC) { b.pristine = common.HexToHash("0x123") },
			deploymentGeneration, deploymentCausePristine, 1, true, true, false, false},
		{"consensus code absent", func(b *deploymentViewRPC) { b.absent[b.consensus] = true },
			deploymentNone, deploymentCauseEmpty, 0, false, false, false, false},
		{"factory code absent", func(b *deploymentViewRPC) { b.absent[b.factory] = true },
			deploymentNone, deploymentCauseEmpty, 0, false, false, false, false},
		{"transition code absent", func(b *deploymentViewRPC) { b.absent[b.transition] = true },
			deploymentNone, deploymentCauseEmpty, 1, true, false, false, false},
		{"late read failure", func(b *deploymentViewRPC) {
			b.rows = alternateDeploymentRows()
			b.count = 3
			b.overrides[testDeploymentRequest(b.factory, testDeploymentRowInput(2))] = deploymentRPCReply{
				err: deploymentRPCError{-32000, "temporary state unavailable"}}
		}, deploymentNone, deploymentCauseNone, 2, true, true, true, true},
		{"revert text is not mismatch", func(b *deploymentViewRPC) {
			b.overrides[testDeploymentRequest(b.transition, testDeploymentSelector(testMarchSignature))] = deploymentRPCReply{
				err: deploymentRPCError{3, "execution reverted: MARCHID incompatible"}}
		}, deploymentNone, deploymentCauseNone, 1, true, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, b := newDeploymentViewRPC(t, canonicalDeploymentRows(), 2)
			tc.change(b)
			evidence, err := adapter.ReadDeployment(t.Context(), b.head)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, evidence.Candidate)
			require.Equal(t, tc.cause, evidence.Cause)
			require.Equal(t, tc.name != "consensus code absent", evidence.FactoryKnown, "retain factory on every early identity/count exit")
			require.Len(t, evidence.Rows, tc.rowCount)
			if tc.pristine {
				require.Equal(t, b.pristine, evidence.Pristine)
			} else {
				require.Zero(t, evidence.Pristine)
			}
			if tc.count {
				require.Equal(t, b.count, evidence.Count)
			} else {
				require.Zero(t, evidence.Count)
			}
			if evidence.FactoryKnown {
				require.Equal(t, b.factory, evidence.Factory)
			}
			if tc.transition {
				require.Equal(t, b.transition, evidence.Transition)
			}
		})
	}
	t.Run("unsupported code capability", func(t *testing.T) {
		adapter, b := newDeploymentViewRPC(t, canonicalDeploymentRows(), 2)
		request := testDeploymentRequest(b.consensus, testDeploymentSelector(testFactorySignature))
		b.overrides[request] = deploymentRPCReply{err: deploymentRPCError{-32601, "method eth_call unavailable"}}
		evidence, err := adapter.ReadDeployment(t.Context(), b.head)
		require.ErrorIs(t, err, txsender.ErrUnsupported)
		require.True(t, txsender.PassInvalid(err))
		require.False(t, txsender.HardFailure(err))
		require.Equal(t, deploymentNone, evidence.Candidate)
		require.Nil(t, evidence.Malformed)
	})
	t.Run("hash required", func(t *testing.T) {
		adapter, b := newDeploymentViewRPC(t, canonicalDeploymentRows(), 2)
		evidence, err := adapter.ReadDeployment(t.Context(), txsender.BlockRef{Number: b.head.Number})
		require.ErrorIs(t, err, txsender.ErrInconclusive)
		require.Equal(t, deploymentNone, evidence.Candidate)
		require.Empty(t, b.records)
	})
}

func TestDeploymentReaderTypedCapabilityAndAmbiguousViewErrors(t *testing.T) {
	for _, tc := range []struct {
		name        string
		code        int
		message     string
		unsupported bool
	}{
		{"missing RPC method", -32601, "method unavailable", true},
		{"ambiguous schema failure", -32602, "unknown field requireCanonical", false},
		{"revert mentioning capability", 3, "execution reverted: requireCanonical unsupported", false},
		{"read error mentioning mismatch", -32000, "MARCHID incompatible", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, b := newDeploymentViewRPC(t, canonicalDeploymentRows(), 2)
			b.overrides[testDeploymentRequest(b.consensus, testDeploymentSelector(testFactorySignature))] = deploymentRPCReply{
				err: deploymentRPCError{tc.code, tc.message},
			}
			e, err := adapter.ReadDeployment(t.Context(), b.head)
			require.Error(t, err)
			require.Equal(t, "none", string(e.Candidate))
			require.False(t, txsender.HardFailure(err))
			require.Equal(t, tc.unsupported, errors.Is(err, txsender.ErrUnsupported))
			require.Equal(t, tc.unsupported, txsender.PassInvalid(err))
			var rpcError rpc.Error
			require.ErrorAs(t, err, &rpcError)
			require.Equal(t, tc.code, rpcError.ErrorCode())
		})
	}
}
