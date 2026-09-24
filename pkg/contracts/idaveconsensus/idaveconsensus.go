// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package idaveconsensus

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// LeafProof is an auto generated low-level Go binding around an user-defined struct.
type LeafProof struct {
	DataBlock [32]byte
	Siblings  [][32]byte
}

// MachineValidityProof is an auto generated low-level Go binding around an user-defined struct.
type MachineValidityProof struct {
	IflagsYProof    LeafProof
	HtifTohostProof LeafProof
	TxBufferProof   LeafProof
}

// IDaveConsensusMetaData contains all meta data concerning the IDaveConsensus contract.
var IDaveConsensusMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"acceptStagedTournamentResult\",\"inputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"canAcceptStagedTournamentResult\",\"inputs\":[],\"outputs\":[{\"name\":\"isTournamentResultStaged\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"doAllSentriesAgreeWithStagedTournamentResult\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"isClaimStagingPeriodOver\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"epochNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"stagedPostEpochMachineStateHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"stagedPostEpochOutputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"canStageTournamentResult\",\"inputs\":[],\"outputs\":[{\"name\":\"isFinished\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"isTournamentFailed\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"isTournamentResultStaged\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"epochNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"winnerCommitment\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"winnerPostEpochMachineStateHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getApplicationContract\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getClaimStagingPeriod\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getCurrentSealedEpoch\",\"inputs\":[],\"outputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"inputIndexLowerBound\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"inputIndexUpperBound\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"tournament\",\"type\":\"address\",\"internalType\":\"contractITournament\"},{\"name\":\"isTournamentResultStaged\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"stagingBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"stagedPostEpochMachineStateHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"stagedPostEpochOutputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDeploymentBlockNumber\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getInputBox\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractIInputBox\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getLastFinalizedMachineMerkleRoot\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNumberOfSentries\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getSentryById\",\"inputs\":[{\"name\":\"sentryId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getSentryClaimCount\",\"inputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"postEpochMachineStateHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getSentryId\",\"inputs\":[{\"name\":\"sentry\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getSentryManager\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getTournamentFactory\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractITournamentFactory\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"hasSentryClaimedInEpoch\",\"inputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"sentryId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isOutputsMerkleRootValid\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"provideMerkleRootOfInput\",\"inputs\":[{\"name\":\"inputIndexWithinEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"input\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"rotateSentry\",\"inputs\":[{\"name\":\"currentSentry\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newSentry\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"stageTournamentResult\",\"inputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"proof\",\"type\":\"tuple\",\"internalType\":\"structMachineValidityProof\",\"components\":[{\"name\":\"iflagsYProof\",\"type\":\"tuple\",\"internalType\":\"structLeafProof\",\"components\":[{\"name\":\"dataBlock\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"siblings\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}]},{\"name\":\"htifTohostProof\",\"type\":\"tuple\",\"internalType\":\"structLeafProof\",\"components\":[{\"name\":\"dataBlock\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"siblings\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}]},{\"name\":\"txBufferProof\",\"type\":\"tuple\",\"internalType\":\"structLeafProof\",\"components\":[{\"name\":\"dataBlock\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"siblings\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}]}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"submitSentryClaim\",\"inputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"postEpochMachineStateHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"supportsInterface\",\"inputs\":[{\"name\":\"interfaceId\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"wasInputFinalized\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"inputIndex\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"blockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"ConsensusCreation\",\"inputs\":[{\"name\":\"inputBox\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractIInputBox\"},{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"tournamentFactory\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractITournamentFactory\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"EpochSealed\",\"inputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"inputIndexLowerBound\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"inputIndexUpperBound\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"initialMachineStateHash\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Machine.Hash\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"},{\"name\":\"tournament\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractITournament\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"EpochStaged\",\"inputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"stagedPostEpochMachineStateHash\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Machine.Hash\"},{\"name\":\"stagedPostEpochOutputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SentryClaim\",\"inputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"sentryId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"sentry\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"postEpochMachineStateHash\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Machine.Hash\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SentryRotation\",\"inputs\":[{\"name\":\"sentryId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"oldSentry\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newSentry\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"ApplicationForeclosed\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ApplicationMismatch\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"received\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ApplicationNotDeployed\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ApplicationReverted\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"error\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"type\":\"error\",\"name\":\"CallerIsNotSentry\",\"inputs\":[{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"CallerIsNotSentryManager\",\"inputs\":[{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"CannotRotateNonSentry\",\"inputs\":[{\"name\":\"nonSentry\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ClaimStagingPeriodNotOverYet\",\"inputs\":[{\"name\":\"numberOfBlocksAfterStaging\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"claimStagingPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataBlockTooLarge\",\"inputs\":[{\"name\":\"log2DataBlockSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxLog2DataBlockSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DriveSmallerThanData\",\"inputs\":[{\"name\":\"driveSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"dataSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DriveSmallerThanDataBlock\",\"inputs\":[{\"name\":\"log2DriveSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"log2DataBlockSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DriveTooLarge\",\"inputs\":[{\"name\":\"log2DriveSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxLog2DriveSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DuplicatedSentryAddress\",\"inputs\":[{\"name\":\"sentryId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"sentry\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"IllformedApplicationReturnData\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"type\":\"error\",\"name\":\"IncorrectEpochNumber\",\"inputs\":[{\"name\":\"received\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InputBoxNotDeployed\",\"inputs\":[{\"name\":\"inputBox\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"InputHashMismatch\",\"inputs\":[{\"name\":\"fromReceivedInput\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"fromInputBox\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"type\":\"error\",\"name\":\"InvalidMachineMerkleProof\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidNodeIndex\",\"inputs\":[{\"name\":\"nodeIndex\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"height\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidPostEpochMachineHtifTohostRegister\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidPostEpochMachineIflagsYRegister\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidSiblingsArrayLength\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SentryAlreadyClaimed\",\"inputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"sentryId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"TournamentNotFinishedYet\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TournamentResultAlreadyStaged\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TournamentResultNotStaged\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"UnexpectedFinalStackDepth\",\"inputs\":[{\"name\":\"stackDepth\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ZeroSentryAddress\",\"inputs\":[]}]",
}

// IDaveConsensusABI is the input ABI used to generate the binding from.
// Deprecated: Use IDaveConsensusMetaData.ABI instead.
var IDaveConsensusABI = IDaveConsensusMetaData.ABI

// IDaveConsensus is an auto generated Go binding around an Ethereum contract.
type IDaveConsensus struct {
	IDaveConsensusCaller     // Read-only binding to the contract
	IDaveConsensusTransactor // Write-only binding to the contract
	IDaveConsensusFilterer   // Log filterer for contract events
}

// IDaveConsensusCaller is an auto generated read-only Go binding around an Ethereum contract.
type IDaveConsensusCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IDaveConsensusTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IDaveConsensusTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IDaveConsensusFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IDaveConsensusFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IDaveConsensusSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IDaveConsensusSession struct {
	Contract     *IDaveConsensus   // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IDaveConsensusCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IDaveConsensusCallerSession struct {
	Contract *IDaveConsensusCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts         // Call options to use throughout this session
}

// IDaveConsensusTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IDaveConsensusTransactorSession struct {
	Contract     *IDaveConsensusTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts         // Transaction auth options to use throughout this session
}

// IDaveConsensusRaw is an auto generated low-level Go binding around an Ethereum contract.
type IDaveConsensusRaw struct {
	Contract *IDaveConsensus // Generic contract binding to access the raw methods on
}

// IDaveConsensusCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IDaveConsensusCallerRaw struct {
	Contract *IDaveConsensusCaller // Generic read-only contract binding to access the raw methods on
}

// IDaveConsensusTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IDaveConsensusTransactorRaw struct {
	Contract *IDaveConsensusTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIDaveConsensus creates a new instance of IDaveConsensus, bound to a specific deployed contract.
func NewIDaveConsensus(address common.Address, backend bind.ContractBackend) (*IDaveConsensus, error) {
	contract, err := bindIDaveConsensus(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IDaveConsensus{IDaveConsensusCaller: IDaveConsensusCaller{contract: contract}, IDaveConsensusTransactor: IDaveConsensusTransactor{contract: contract}, IDaveConsensusFilterer: IDaveConsensusFilterer{contract: contract}}, nil
}

// NewIDaveConsensusCaller creates a new read-only instance of IDaveConsensus, bound to a specific deployed contract.
func NewIDaveConsensusCaller(address common.Address, caller bind.ContractCaller) (*IDaveConsensusCaller, error) {
	contract, err := bindIDaveConsensus(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IDaveConsensusCaller{contract: contract}, nil
}

// NewIDaveConsensusTransactor creates a new write-only instance of IDaveConsensus, bound to a specific deployed contract.
func NewIDaveConsensusTransactor(address common.Address, transactor bind.ContractTransactor) (*IDaveConsensusTransactor, error) {
	contract, err := bindIDaveConsensus(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IDaveConsensusTransactor{contract: contract}, nil
}

// NewIDaveConsensusFilterer creates a new log filterer instance of IDaveConsensus, bound to a specific deployed contract.
func NewIDaveConsensusFilterer(address common.Address, filterer bind.ContractFilterer) (*IDaveConsensusFilterer, error) {
	contract, err := bindIDaveConsensus(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IDaveConsensusFilterer{contract: contract}, nil
}

// bindIDaveConsensus binds a generic wrapper to an already deployed contract.
func bindIDaveConsensus(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IDaveConsensusMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IDaveConsensus *IDaveConsensusRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IDaveConsensus.Contract.IDaveConsensusCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IDaveConsensus *IDaveConsensusRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.IDaveConsensusTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IDaveConsensus *IDaveConsensusRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.IDaveConsensusTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IDaveConsensus *IDaveConsensusCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IDaveConsensus.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IDaveConsensus *IDaveConsensusTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IDaveConsensus *IDaveConsensusTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.contract.Transact(opts, method, params...)
}

// CanAcceptStagedTournamentResult is a free data retrieval call binding the contract method 0xab36a12e.
//
// Solidity: function canAcceptStagedTournamentResult() view returns(bool isTournamentResultStaged, bool doAllSentriesAgreeWithStagedTournamentResult, bool isClaimStagingPeriodOver, uint256 epochNumber, bytes32 stagedPostEpochMachineStateHash, bytes32 stagedPostEpochOutputsMerkleRoot)
func (_IDaveConsensus *IDaveConsensusCaller) CanAcceptStagedTournamentResult(opts *bind.CallOpts) (struct {
	IsTournamentResultStaged                     bool
	DoAllSentriesAgreeWithStagedTournamentResult bool
	IsClaimStagingPeriodOver                     bool
	EpochNumber                                  *big.Int
	StagedPostEpochMachineStateHash              [32]byte
	StagedPostEpochOutputsMerkleRoot             [32]byte
}, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "canAcceptStagedTournamentResult")

	outstruct := new(struct {
		IsTournamentResultStaged                     bool
		DoAllSentriesAgreeWithStagedTournamentResult bool
		IsClaimStagingPeriodOver                     bool
		EpochNumber                                  *big.Int
		StagedPostEpochMachineStateHash              [32]byte
		StagedPostEpochOutputsMerkleRoot             [32]byte
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.IsTournamentResultStaged = *abi.ConvertType(out[0], new(bool)).(*bool)
	outstruct.DoAllSentriesAgreeWithStagedTournamentResult = *abi.ConvertType(out[1], new(bool)).(*bool)
	outstruct.IsClaimStagingPeriodOver = *abi.ConvertType(out[2], new(bool)).(*bool)
	outstruct.EpochNumber = *abi.ConvertType(out[3], new(*big.Int)).(**big.Int)
	outstruct.StagedPostEpochMachineStateHash = *abi.ConvertType(out[4], new([32]byte)).(*[32]byte)
	outstruct.StagedPostEpochOutputsMerkleRoot = *abi.ConvertType(out[5], new([32]byte)).(*[32]byte)

	return *outstruct, err

}

// CanAcceptStagedTournamentResult is a free data retrieval call binding the contract method 0xab36a12e.
//
// Solidity: function canAcceptStagedTournamentResult() view returns(bool isTournamentResultStaged, bool doAllSentriesAgreeWithStagedTournamentResult, bool isClaimStagingPeriodOver, uint256 epochNumber, bytes32 stagedPostEpochMachineStateHash, bytes32 stagedPostEpochOutputsMerkleRoot)
func (_IDaveConsensus *IDaveConsensusSession) CanAcceptStagedTournamentResult() (struct {
	IsTournamentResultStaged                     bool
	DoAllSentriesAgreeWithStagedTournamentResult bool
	IsClaimStagingPeriodOver                     bool
	EpochNumber                                  *big.Int
	StagedPostEpochMachineStateHash              [32]byte
	StagedPostEpochOutputsMerkleRoot             [32]byte
}, error) {
	return _IDaveConsensus.Contract.CanAcceptStagedTournamentResult(&_IDaveConsensus.CallOpts)
}

// CanAcceptStagedTournamentResult is a free data retrieval call binding the contract method 0xab36a12e.
//
// Solidity: function canAcceptStagedTournamentResult() view returns(bool isTournamentResultStaged, bool doAllSentriesAgreeWithStagedTournamentResult, bool isClaimStagingPeriodOver, uint256 epochNumber, bytes32 stagedPostEpochMachineStateHash, bytes32 stagedPostEpochOutputsMerkleRoot)
func (_IDaveConsensus *IDaveConsensusCallerSession) CanAcceptStagedTournamentResult() (struct {
	IsTournamentResultStaged                     bool
	DoAllSentriesAgreeWithStagedTournamentResult bool
	IsClaimStagingPeriodOver                     bool
	EpochNumber                                  *big.Int
	StagedPostEpochMachineStateHash              [32]byte
	StagedPostEpochOutputsMerkleRoot             [32]byte
}, error) {
	return _IDaveConsensus.Contract.CanAcceptStagedTournamentResult(&_IDaveConsensus.CallOpts)
}

// CanStageTournamentResult is a free data retrieval call binding the contract method 0x5c2694fd.
//
// Solidity: function canStageTournamentResult() view returns(bool isFinished, bool isTournamentFailed, bool isTournamentResultStaged, uint256 epochNumber, bytes32 winnerCommitment, bytes32 winnerPostEpochMachineStateHash)
func (_IDaveConsensus *IDaveConsensusCaller) CanStageTournamentResult(opts *bind.CallOpts) (struct {
	IsFinished                      bool
	IsTournamentFailed              bool
	IsTournamentResultStaged        bool
	EpochNumber                     *big.Int
	WinnerCommitment                [32]byte
	WinnerPostEpochMachineStateHash [32]byte
}, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "canStageTournamentResult")

	outstruct := new(struct {
		IsFinished                      bool
		IsTournamentFailed              bool
		IsTournamentResultStaged        bool
		EpochNumber                     *big.Int
		WinnerCommitment                [32]byte
		WinnerPostEpochMachineStateHash [32]byte
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.IsFinished = *abi.ConvertType(out[0], new(bool)).(*bool)
	outstruct.IsTournamentFailed = *abi.ConvertType(out[1], new(bool)).(*bool)
	outstruct.IsTournamentResultStaged = *abi.ConvertType(out[2], new(bool)).(*bool)
	outstruct.EpochNumber = *abi.ConvertType(out[3], new(*big.Int)).(**big.Int)
	outstruct.WinnerCommitment = *abi.ConvertType(out[4], new([32]byte)).(*[32]byte)
	outstruct.WinnerPostEpochMachineStateHash = *abi.ConvertType(out[5], new([32]byte)).(*[32]byte)

	return *outstruct, err

}

// CanStageTournamentResult is a free data retrieval call binding the contract method 0x5c2694fd.
//
// Solidity: function canStageTournamentResult() view returns(bool isFinished, bool isTournamentFailed, bool isTournamentResultStaged, uint256 epochNumber, bytes32 winnerCommitment, bytes32 winnerPostEpochMachineStateHash)
func (_IDaveConsensus *IDaveConsensusSession) CanStageTournamentResult() (struct {
	IsFinished                      bool
	IsTournamentFailed              bool
	IsTournamentResultStaged        bool
	EpochNumber                     *big.Int
	WinnerCommitment                [32]byte
	WinnerPostEpochMachineStateHash [32]byte
}, error) {
	return _IDaveConsensus.Contract.CanStageTournamentResult(&_IDaveConsensus.CallOpts)
}

// CanStageTournamentResult is a free data retrieval call binding the contract method 0x5c2694fd.
//
// Solidity: function canStageTournamentResult() view returns(bool isFinished, bool isTournamentFailed, bool isTournamentResultStaged, uint256 epochNumber, bytes32 winnerCommitment, bytes32 winnerPostEpochMachineStateHash)
func (_IDaveConsensus *IDaveConsensusCallerSession) CanStageTournamentResult() (struct {
	IsFinished                      bool
	IsTournamentFailed              bool
	IsTournamentResultStaged        bool
	EpochNumber                     *big.Int
	WinnerCommitment                [32]byte
	WinnerPostEpochMachineStateHash [32]byte
}, error) {
	return _IDaveConsensus.Contract.CanStageTournamentResult(&_IDaveConsensus.CallOpts)
}

// GetApplicationContract is a free data retrieval call binding the contract method 0xc050be00.
//
// Solidity: function getApplicationContract() view returns(address)
func (_IDaveConsensus *IDaveConsensusCaller) GetApplicationContract(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getApplicationContract")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetApplicationContract is a free data retrieval call binding the contract method 0xc050be00.
//
// Solidity: function getApplicationContract() view returns(address)
func (_IDaveConsensus *IDaveConsensusSession) GetApplicationContract() (common.Address, error) {
	return _IDaveConsensus.Contract.GetApplicationContract(&_IDaveConsensus.CallOpts)
}

// GetApplicationContract is a free data retrieval call binding the contract method 0xc050be00.
//
// Solidity: function getApplicationContract() view returns(address)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetApplicationContract() (common.Address, error) {
	return _IDaveConsensus.Contract.GetApplicationContract(&_IDaveConsensus.CallOpts)
}

// GetClaimStagingPeriod is a free data retrieval call binding the contract method 0xa04c6564.
//
// Solidity: function getClaimStagingPeriod() view returns(uint256)
func (_IDaveConsensus *IDaveConsensusCaller) GetClaimStagingPeriod(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getClaimStagingPeriod")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetClaimStagingPeriod is a free data retrieval call binding the contract method 0xa04c6564.
//
// Solidity: function getClaimStagingPeriod() view returns(uint256)
func (_IDaveConsensus *IDaveConsensusSession) GetClaimStagingPeriod() (*big.Int, error) {
	return _IDaveConsensus.Contract.GetClaimStagingPeriod(&_IDaveConsensus.CallOpts)
}

// GetClaimStagingPeriod is a free data retrieval call binding the contract method 0xa04c6564.
//
// Solidity: function getClaimStagingPeriod() view returns(uint256)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetClaimStagingPeriod() (*big.Int, error) {
	return _IDaveConsensus.Contract.GetClaimStagingPeriod(&_IDaveConsensus.CallOpts)
}

// GetCurrentSealedEpoch is a free data retrieval call binding the contract method 0x1239acd9.
//
// Solidity: function getCurrentSealedEpoch() view returns(uint256 epochNumber, uint256 inputIndexLowerBound, uint256 inputIndexUpperBound, address tournament, bool isTournamentResultStaged, uint256 stagingBlockNumber, bytes32 stagedPostEpochMachineStateHash, bytes32 stagedPostEpochOutputsMerkleRoot)
func (_IDaveConsensus *IDaveConsensusCaller) GetCurrentSealedEpoch(opts *bind.CallOpts) (struct {
	EpochNumber                      *big.Int
	InputIndexLowerBound             *big.Int
	InputIndexUpperBound             *big.Int
	Tournament                       common.Address
	IsTournamentResultStaged         bool
	StagingBlockNumber               *big.Int
	StagedPostEpochMachineStateHash  [32]byte
	StagedPostEpochOutputsMerkleRoot [32]byte
}, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getCurrentSealedEpoch")

	outstruct := new(struct {
		EpochNumber                      *big.Int
		InputIndexLowerBound             *big.Int
		InputIndexUpperBound             *big.Int
		Tournament                       common.Address
		IsTournamentResultStaged         bool
		StagingBlockNumber               *big.Int
		StagedPostEpochMachineStateHash  [32]byte
		StagedPostEpochOutputsMerkleRoot [32]byte
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.EpochNumber = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.InputIndexLowerBound = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)
	outstruct.InputIndexUpperBound = *abi.ConvertType(out[2], new(*big.Int)).(**big.Int)
	outstruct.Tournament = *abi.ConvertType(out[3], new(common.Address)).(*common.Address)
	outstruct.IsTournamentResultStaged = *abi.ConvertType(out[4], new(bool)).(*bool)
	outstruct.StagingBlockNumber = *abi.ConvertType(out[5], new(*big.Int)).(**big.Int)
	outstruct.StagedPostEpochMachineStateHash = *abi.ConvertType(out[6], new([32]byte)).(*[32]byte)
	outstruct.StagedPostEpochOutputsMerkleRoot = *abi.ConvertType(out[7], new([32]byte)).(*[32]byte)

	return *outstruct, err

}

// GetCurrentSealedEpoch is a free data retrieval call binding the contract method 0x1239acd9.
//
// Solidity: function getCurrentSealedEpoch() view returns(uint256 epochNumber, uint256 inputIndexLowerBound, uint256 inputIndexUpperBound, address tournament, bool isTournamentResultStaged, uint256 stagingBlockNumber, bytes32 stagedPostEpochMachineStateHash, bytes32 stagedPostEpochOutputsMerkleRoot)
func (_IDaveConsensus *IDaveConsensusSession) GetCurrentSealedEpoch() (struct {
	EpochNumber                      *big.Int
	InputIndexLowerBound             *big.Int
	InputIndexUpperBound             *big.Int
	Tournament                       common.Address
	IsTournamentResultStaged         bool
	StagingBlockNumber               *big.Int
	StagedPostEpochMachineStateHash  [32]byte
	StagedPostEpochOutputsMerkleRoot [32]byte
}, error) {
	return _IDaveConsensus.Contract.GetCurrentSealedEpoch(&_IDaveConsensus.CallOpts)
}

// GetCurrentSealedEpoch is a free data retrieval call binding the contract method 0x1239acd9.
//
// Solidity: function getCurrentSealedEpoch() view returns(uint256 epochNumber, uint256 inputIndexLowerBound, uint256 inputIndexUpperBound, address tournament, bool isTournamentResultStaged, uint256 stagingBlockNumber, bytes32 stagedPostEpochMachineStateHash, bytes32 stagedPostEpochOutputsMerkleRoot)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetCurrentSealedEpoch() (struct {
	EpochNumber                      *big.Int
	InputIndexLowerBound             *big.Int
	InputIndexUpperBound             *big.Int
	Tournament                       common.Address
	IsTournamentResultStaged         bool
	StagingBlockNumber               *big.Int
	StagedPostEpochMachineStateHash  [32]byte
	StagedPostEpochOutputsMerkleRoot [32]byte
}, error) {
	return _IDaveConsensus.Contract.GetCurrentSealedEpoch(&_IDaveConsensus.CallOpts)
}

// GetDeploymentBlockNumber is a free data retrieval call binding the contract method 0xb3a1acd8.
//
// Solidity: function getDeploymentBlockNumber() view returns(uint256)
func (_IDaveConsensus *IDaveConsensusCaller) GetDeploymentBlockNumber(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getDeploymentBlockNumber")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetDeploymentBlockNumber is a free data retrieval call binding the contract method 0xb3a1acd8.
//
// Solidity: function getDeploymentBlockNumber() view returns(uint256)
func (_IDaveConsensus *IDaveConsensusSession) GetDeploymentBlockNumber() (*big.Int, error) {
	return _IDaveConsensus.Contract.GetDeploymentBlockNumber(&_IDaveConsensus.CallOpts)
}

// GetDeploymentBlockNumber is a free data retrieval call binding the contract method 0xb3a1acd8.
//
// Solidity: function getDeploymentBlockNumber() view returns(uint256)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetDeploymentBlockNumber() (*big.Int, error) {
	return _IDaveConsensus.Contract.GetDeploymentBlockNumber(&_IDaveConsensus.CallOpts)
}

// GetInputBox is a free data retrieval call binding the contract method 0x00aace9a.
//
// Solidity: function getInputBox() view returns(address)
func (_IDaveConsensus *IDaveConsensusCaller) GetInputBox(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getInputBox")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetInputBox is a free data retrieval call binding the contract method 0x00aace9a.
//
// Solidity: function getInputBox() view returns(address)
func (_IDaveConsensus *IDaveConsensusSession) GetInputBox() (common.Address, error) {
	return _IDaveConsensus.Contract.GetInputBox(&_IDaveConsensus.CallOpts)
}

// GetInputBox is a free data retrieval call binding the contract method 0x00aace9a.
//
// Solidity: function getInputBox() view returns(address)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetInputBox() (common.Address, error) {
	return _IDaveConsensus.Contract.GetInputBox(&_IDaveConsensus.CallOpts)
}

// GetLastFinalizedMachineMerkleRoot is a free data retrieval call binding the contract method 0x5ac9cfbf.
//
// Solidity: function getLastFinalizedMachineMerkleRoot(address appContract) view returns(bytes32)
func (_IDaveConsensus *IDaveConsensusCaller) GetLastFinalizedMachineMerkleRoot(opts *bind.CallOpts, appContract common.Address) ([32]byte, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getLastFinalizedMachineMerkleRoot", appContract)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetLastFinalizedMachineMerkleRoot is a free data retrieval call binding the contract method 0x5ac9cfbf.
//
// Solidity: function getLastFinalizedMachineMerkleRoot(address appContract) view returns(bytes32)
func (_IDaveConsensus *IDaveConsensusSession) GetLastFinalizedMachineMerkleRoot(appContract common.Address) ([32]byte, error) {
	return _IDaveConsensus.Contract.GetLastFinalizedMachineMerkleRoot(&_IDaveConsensus.CallOpts, appContract)
}

// GetLastFinalizedMachineMerkleRoot is a free data retrieval call binding the contract method 0x5ac9cfbf.
//
// Solidity: function getLastFinalizedMachineMerkleRoot(address appContract) view returns(bytes32)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetLastFinalizedMachineMerkleRoot(appContract common.Address) ([32]byte, error) {
	return _IDaveConsensus.Contract.GetLastFinalizedMachineMerkleRoot(&_IDaveConsensus.CallOpts, appContract)
}

// GetNumberOfSentries is a free data retrieval call binding the contract method 0xf2f0143d.
//
// Solidity: function getNumberOfSentries() view returns(uint256)
func (_IDaveConsensus *IDaveConsensusCaller) GetNumberOfSentries(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getNumberOfSentries")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNumberOfSentries is a free data retrieval call binding the contract method 0xf2f0143d.
//
// Solidity: function getNumberOfSentries() view returns(uint256)
func (_IDaveConsensus *IDaveConsensusSession) GetNumberOfSentries() (*big.Int, error) {
	return _IDaveConsensus.Contract.GetNumberOfSentries(&_IDaveConsensus.CallOpts)
}

// GetNumberOfSentries is a free data retrieval call binding the contract method 0xf2f0143d.
//
// Solidity: function getNumberOfSentries() view returns(uint256)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetNumberOfSentries() (*big.Int, error) {
	return _IDaveConsensus.Contract.GetNumberOfSentries(&_IDaveConsensus.CallOpts)
}

// GetSentryById is a free data retrieval call binding the contract method 0x50c8378d.
//
// Solidity: function getSentryById(uint256 sentryId) view returns(address)
func (_IDaveConsensus *IDaveConsensusCaller) GetSentryById(opts *bind.CallOpts, sentryId *big.Int) (common.Address, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getSentryById", sentryId)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetSentryById is a free data retrieval call binding the contract method 0x50c8378d.
//
// Solidity: function getSentryById(uint256 sentryId) view returns(address)
func (_IDaveConsensus *IDaveConsensusSession) GetSentryById(sentryId *big.Int) (common.Address, error) {
	return _IDaveConsensus.Contract.GetSentryById(&_IDaveConsensus.CallOpts, sentryId)
}

// GetSentryById is a free data retrieval call binding the contract method 0x50c8378d.
//
// Solidity: function getSentryById(uint256 sentryId) view returns(address)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetSentryById(sentryId *big.Int) (common.Address, error) {
	return _IDaveConsensus.Contract.GetSentryById(&_IDaveConsensus.CallOpts, sentryId)
}

// GetSentryClaimCount is a free data retrieval call binding the contract method 0x47d309f7.
//
// Solidity: function getSentryClaimCount(uint256 epochNumber, bytes32 postEpochMachineStateHash) view returns(uint256)
func (_IDaveConsensus *IDaveConsensusCaller) GetSentryClaimCount(opts *bind.CallOpts, epochNumber *big.Int, postEpochMachineStateHash [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getSentryClaimCount", epochNumber, postEpochMachineStateHash)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetSentryClaimCount is a free data retrieval call binding the contract method 0x47d309f7.
//
// Solidity: function getSentryClaimCount(uint256 epochNumber, bytes32 postEpochMachineStateHash) view returns(uint256)
func (_IDaveConsensus *IDaveConsensusSession) GetSentryClaimCount(epochNumber *big.Int, postEpochMachineStateHash [32]byte) (*big.Int, error) {
	return _IDaveConsensus.Contract.GetSentryClaimCount(&_IDaveConsensus.CallOpts, epochNumber, postEpochMachineStateHash)
}

// GetSentryClaimCount is a free data retrieval call binding the contract method 0x47d309f7.
//
// Solidity: function getSentryClaimCount(uint256 epochNumber, bytes32 postEpochMachineStateHash) view returns(uint256)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetSentryClaimCount(epochNumber *big.Int, postEpochMachineStateHash [32]byte) (*big.Int, error) {
	return _IDaveConsensus.Contract.GetSentryClaimCount(&_IDaveConsensus.CallOpts, epochNumber, postEpochMachineStateHash)
}

// GetSentryId is a free data retrieval call binding the contract method 0x8bb6ae16.
//
// Solidity: function getSentryId(address sentry) view returns(uint256)
func (_IDaveConsensus *IDaveConsensusCaller) GetSentryId(opts *bind.CallOpts, sentry common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getSentryId", sentry)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetSentryId is a free data retrieval call binding the contract method 0x8bb6ae16.
//
// Solidity: function getSentryId(address sentry) view returns(uint256)
func (_IDaveConsensus *IDaveConsensusSession) GetSentryId(sentry common.Address) (*big.Int, error) {
	return _IDaveConsensus.Contract.GetSentryId(&_IDaveConsensus.CallOpts, sentry)
}

// GetSentryId is a free data retrieval call binding the contract method 0x8bb6ae16.
//
// Solidity: function getSentryId(address sentry) view returns(uint256)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetSentryId(sentry common.Address) (*big.Int, error) {
	return _IDaveConsensus.Contract.GetSentryId(&_IDaveConsensus.CallOpts, sentry)
}

// GetSentryManager is a free data retrieval call binding the contract method 0x0c2f5d7c.
//
// Solidity: function getSentryManager() view returns(address)
func (_IDaveConsensus *IDaveConsensusCaller) GetSentryManager(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getSentryManager")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetSentryManager is a free data retrieval call binding the contract method 0x0c2f5d7c.
//
// Solidity: function getSentryManager() view returns(address)
func (_IDaveConsensus *IDaveConsensusSession) GetSentryManager() (common.Address, error) {
	return _IDaveConsensus.Contract.GetSentryManager(&_IDaveConsensus.CallOpts)
}

// GetSentryManager is a free data retrieval call binding the contract method 0x0c2f5d7c.
//
// Solidity: function getSentryManager() view returns(address)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetSentryManager() (common.Address, error) {
	return _IDaveConsensus.Contract.GetSentryManager(&_IDaveConsensus.CallOpts)
}

// GetTournamentFactory is a free data retrieval call binding the contract method 0x813a1aaf.
//
// Solidity: function getTournamentFactory() view returns(address)
func (_IDaveConsensus *IDaveConsensusCaller) GetTournamentFactory(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getTournamentFactory")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetTournamentFactory is a free data retrieval call binding the contract method 0x813a1aaf.
//
// Solidity: function getTournamentFactory() view returns(address)
func (_IDaveConsensus *IDaveConsensusSession) GetTournamentFactory() (common.Address, error) {
	return _IDaveConsensus.Contract.GetTournamentFactory(&_IDaveConsensus.CallOpts)
}

// GetTournamentFactory is a free data retrieval call binding the contract method 0x813a1aaf.
//
// Solidity: function getTournamentFactory() view returns(address)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetTournamentFactory() (common.Address, error) {
	return _IDaveConsensus.Contract.GetTournamentFactory(&_IDaveConsensus.CallOpts)
}

// HasSentryClaimedInEpoch is a free data retrieval call binding the contract method 0xb55832c6.
//
// Solidity: function hasSentryClaimedInEpoch(uint256 epochNumber, uint256 sentryId) view returns(bool)
func (_IDaveConsensus *IDaveConsensusCaller) HasSentryClaimedInEpoch(opts *bind.CallOpts, epochNumber *big.Int, sentryId *big.Int) (bool, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "hasSentryClaimedInEpoch", epochNumber, sentryId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// HasSentryClaimedInEpoch is a free data retrieval call binding the contract method 0xb55832c6.
//
// Solidity: function hasSentryClaimedInEpoch(uint256 epochNumber, uint256 sentryId) view returns(bool)
func (_IDaveConsensus *IDaveConsensusSession) HasSentryClaimedInEpoch(epochNumber *big.Int, sentryId *big.Int) (bool, error) {
	return _IDaveConsensus.Contract.HasSentryClaimedInEpoch(&_IDaveConsensus.CallOpts, epochNumber, sentryId)
}

// HasSentryClaimedInEpoch is a free data retrieval call binding the contract method 0xb55832c6.
//
// Solidity: function hasSentryClaimedInEpoch(uint256 epochNumber, uint256 sentryId) view returns(bool)
func (_IDaveConsensus *IDaveConsensusCallerSession) HasSentryClaimedInEpoch(epochNumber *big.Int, sentryId *big.Int) (bool, error) {
	return _IDaveConsensus.Contract.HasSentryClaimedInEpoch(&_IDaveConsensus.CallOpts, epochNumber, sentryId)
}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IDaveConsensus *IDaveConsensusCaller) IsOutputsMerkleRootValid(opts *bind.CallOpts, appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "isOutputsMerkleRootValid", appContract, outputsMerkleRoot)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IDaveConsensus *IDaveConsensusSession) IsOutputsMerkleRootValid(appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	return _IDaveConsensus.Contract.IsOutputsMerkleRootValid(&_IDaveConsensus.CallOpts, appContract, outputsMerkleRoot)
}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IDaveConsensus *IDaveConsensusCallerSession) IsOutputsMerkleRootValid(appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	return _IDaveConsensus.Contract.IsOutputsMerkleRootValid(&_IDaveConsensus.CallOpts, appContract, outputsMerkleRoot)
}

// ProvideMerkleRootOfInput is a free data retrieval call binding the contract method 0x7a96f480.
//
// Solidity: function provideMerkleRootOfInput(uint256 inputIndexWithinEpoch, bytes input) view returns(bytes32)
func (_IDaveConsensus *IDaveConsensusCaller) ProvideMerkleRootOfInput(opts *bind.CallOpts, inputIndexWithinEpoch *big.Int, input []byte) ([32]byte, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "provideMerkleRootOfInput", inputIndexWithinEpoch, input)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// ProvideMerkleRootOfInput is a free data retrieval call binding the contract method 0x7a96f480.
//
// Solidity: function provideMerkleRootOfInput(uint256 inputIndexWithinEpoch, bytes input) view returns(bytes32)
func (_IDaveConsensus *IDaveConsensusSession) ProvideMerkleRootOfInput(inputIndexWithinEpoch *big.Int, input []byte) ([32]byte, error) {
	return _IDaveConsensus.Contract.ProvideMerkleRootOfInput(&_IDaveConsensus.CallOpts, inputIndexWithinEpoch, input)
}

// ProvideMerkleRootOfInput is a free data retrieval call binding the contract method 0x7a96f480.
//
// Solidity: function provideMerkleRootOfInput(uint256 inputIndexWithinEpoch, bytes input) view returns(bytes32)
func (_IDaveConsensus *IDaveConsensusCallerSession) ProvideMerkleRootOfInput(inputIndexWithinEpoch *big.Int, input []byte) ([32]byte, error) {
	return _IDaveConsensus.Contract.ProvideMerkleRootOfInput(&_IDaveConsensus.CallOpts, inputIndexWithinEpoch, input)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IDaveConsensus *IDaveConsensusCaller) SupportsInterface(opts *bind.CallOpts, interfaceId [4]byte) (bool, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "supportsInterface", interfaceId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IDaveConsensus *IDaveConsensusSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _IDaveConsensus.Contract.SupportsInterface(&_IDaveConsensus.CallOpts, interfaceId)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IDaveConsensus *IDaveConsensusCallerSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _IDaveConsensus.Contract.SupportsInterface(&_IDaveConsensus.CallOpts, interfaceId)
}

// WasInputFinalized is a free data retrieval call binding the contract method 0x2b73ad31.
//
// Solidity: function wasInputFinalized(address appContract, uint256 inputIndex, uint256 blockNumber) view returns(bool)
func (_IDaveConsensus *IDaveConsensusCaller) WasInputFinalized(opts *bind.CallOpts, appContract common.Address, inputIndex *big.Int, blockNumber *big.Int) (bool, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "wasInputFinalized", appContract, inputIndex, blockNumber)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// WasInputFinalized is a free data retrieval call binding the contract method 0x2b73ad31.
//
// Solidity: function wasInputFinalized(address appContract, uint256 inputIndex, uint256 blockNumber) view returns(bool)
func (_IDaveConsensus *IDaveConsensusSession) WasInputFinalized(appContract common.Address, inputIndex *big.Int, blockNumber *big.Int) (bool, error) {
	return _IDaveConsensus.Contract.WasInputFinalized(&_IDaveConsensus.CallOpts, appContract, inputIndex, blockNumber)
}

// WasInputFinalized is a free data retrieval call binding the contract method 0x2b73ad31.
//
// Solidity: function wasInputFinalized(address appContract, uint256 inputIndex, uint256 blockNumber) view returns(bool)
func (_IDaveConsensus *IDaveConsensusCallerSession) WasInputFinalized(appContract common.Address, inputIndex *big.Int, blockNumber *big.Int) (bool, error) {
	return _IDaveConsensus.Contract.WasInputFinalized(&_IDaveConsensus.CallOpts, appContract, inputIndex, blockNumber)
}

// AcceptStagedTournamentResult is a paid mutator transaction binding the contract method 0x592b96c4.
//
// Solidity: function acceptStagedTournamentResult(uint256 epochNumber) returns()
func (_IDaveConsensus *IDaveConsensusTransactor) AcceptStagedTournamentResult(opts *bind.TransactOpts, epochNumber *big.Int) (*types.Transaction, error) {
	return _IDaveConsensus.contract.Transact(opts, "acceptStagedTournamentResult", epochNumber)
}

// AcceptStagedTournamentResult is a paid mutator transaction binding the contract method 0x592b96c4.
//
// Solidity: function acceptStagedTournamentResult(uint256 epochNumber) returns()
func (_IDaveConsensus *IDaveConsensusSession) AcceptStagedTournamentResult(epochNumber *big.Int) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.AcceptStagedTournamentResult(&_IDaveConsensus.TransactOpts, epochNumber)
}

// AcceptStagedTournamentResult is a paid mutator transaction binding the contract method 0x592b96c4.
//
// Solidity: function acceptStagedTournamentResult(uint256 epochNumber) returns()
func (_IDaveConsensus *IDaveConsensusTransactorSession) AcceptStagedTournamentResult(epochNumber *big.Int) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.AcceptStagedTournamentResult(&_IDaveConsensus.TransactOpts, epochNumber)
}

// RotateSentry is a paid mutator transaction binding the contract method 0xbf4e5230.
//
// Solidity: function rotateSentry(address currentSentry, address newSentry) returns()
func (_IDaveConsensus *IDaveConsensusTransactor) RotateSentry(opts *bind.TransactOpts, currentSentry common.Address, newSentry common.Address) (*types.Transaction, error) {
	return _IDaveConsensus.contract.Transact(opts, "rotateSentry", currentSentry, newSentry)
}

// RotateSentry is a paid mutator transaction binding the contract method 0xbf4e5230.
//
// Solidity: function rotateSentry(address currentSentry, address newSentry) returns()
func (_IDaveConsensus *IDaveConsensusSession) RotateSentry(currentSentry common.Address, newSentry common.Address) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.RotateSentry(&_IDaveConsensus.TransactOpts, currentSentry, newSentry)
}

// RotateSentry is a paid mutator transaction binding the contract method 0xbf4e5230.
//
// Solidity: function rotateSentry(address currentSentry, address newSentry) returns()
func (_IDaveConsensus *IDaveConsensusTransactorSession) RotateSentry(currentSentry common.Address, newSentry common.Address) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.RotateSentry(&_IDaveConsensus.TransactOpts, currentSentry, newSentry)
}

// StageTournamentResult is a paid mutator transaction binding the contract method 0xc1dd774d.
//
// Solidity: function stageTournamentResult(uint256 epochNumber, ((bytes32,bytes32[]),(bytes32,bytes32[]),(bytes32,bytes32[])) proof) returns()
func (_IDaveConsensus *IDaveConsensusTransactor) StageTournamentResult(opts *bind.TransactOpts, epochNumber *big.Int, proof MachineValidityProof) (*types.Transaction, error) {
	return _IDaveConsensus.contract.Transact(opts, "stageTournamentResult", epochNumber, proof)
}

// StageTournamentResult is a paid mutator transaction binding the contract method 0xc1dd774d.
//
// Solidity: function stageTournamentResult(uint256 epochNumber, ((bytes32,bytes32[]),(bytes32,bytes32[]),(bytes32,bytes32[])) proof) returns()
func (_IDaveConsensus *IDaveConsensusSession) StageTournamentResult(epochNumber *big.Int, proof MachineValidityProof) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.StageTournamentResult(&_IDaveConsensus.TransactOpts, epochNumber, proof)
}

// StageTournamentResult is a paid mutator transaction binding the contract method 0xc1dd774d.
//
// Solidity: function stageTournamentResult(uint256 epochNumber, ((bytes32,bytes32[]),(bytes32,bytes32[]),(bytes32,bytes32[])) proof) returns()
func (_IDaveConsensus *IDaveConsensusTransactorSession) StageTournamentResult(epochNumber *big.Int, proof MachineValidityProof) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.StageTournamentResult(&_IDaveConsensus.TransactOpts, epochNumber, proof)
}

// SubmitSentryClaim is a paid mutator transaction binding the contract method 0x9dab5b26.
//
// Solidity: function submitSentryClaim(uint256 epochNumber, bytes32 postEpochMachineStateHash) returns()
func (_IDaveConsensus *IDaveConsensusTransactor) SubmitSentryClaim(opts *bind.TransactOpts, epochNumber *big.Int, postEpochMachineStateHash [32]byte) (*types.Transaction, error) {
	return _IDaveConsensus.contract.Transact(opts, "submitSentryClaim", epochNumber, postEpochMachineStateHash)
}

// SubmitSentryClaim is a paid mutator transaction binding the contract method 0x9dab5b26.
//
// Solidity: function submitSentryClaim(uint256 epochNumber, bytes32 postEpochMachineStateHash) returns()
func (_IDaveConsensus *IDaveConsensusSession) SubmitSentryClaim(epochNumber *big.Int, postEpochMachineStateHash [32]byte) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.SubmitSentryClaim(&_IDaveConsensus.TransactOpts, epochNumber, postEpochMachineStateHash)
}

// SubmitSentryClaim is a paid mutator transaction binding the contract method 0x9dab5b26.
//
// Solidity: function submitSentryClaim(uint256 epochNumber, bytes32 postEpochMachineStateHash) returns()
func (_IDaveConsensus *IDaveConsensusTransactorSession) SubmitSentryClaim(epochNumber *big.Int, postEpochMachineStateHash [32]byte) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.SubmitSentryClaim(&_IDaveConsensus.TransactOpts, epochNumber, postEpochMachineStateHash)
}

// IDaveConsensusConsensusCreationIterator is returned from FilterConsensusCreation and is used to iterate over the raw logs and unpacked data for ConsensusCreation events raised by the IDaveConsensus contract.
type IDaveConsensusConsensusCreationIterator struct {
	Event *IDaveConsensusConsensusCreation // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IDaveConsensusConsensusCreationIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IDaveConsensusConsensusCreation)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IDaveConsensusConsensusCreation)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IDaveConsensusConsensusCreationIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IDaveConsensusConsensusCreationIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IDaveConsensusConsensusCreation represents a ConsensusCreation event raised by the IDaveConsensus contract.
type IDaveConsensusConsensusCreation struct {
	InputBox          common.Address
	AppContract       common.Address
	TournamentFactory common.Address
	Raw               types.Log // Blockchain specific contextual infos
}

// FilterConsensusCreation is a free log retrieval operation binding the contract event 0xaf68463e16cb5595a44214bea8d366ecf7cd3410269c50f92c104b50a7829daa.
//
// Solidity: event ConsensusCreation(address inputBox, address appContract, address tournamentFactory)
func (_IDaveConsensus *IDaveConsensusFilterer) FilterConsensusCreation(opts *bind.FilterOpts) (*IDaveConsensusConsensusCreationIterator, error) {

	logs, sub, err := _IDaveConsensus.contract.FilterLogs(opts, "ConsensusCreation")
	if err != nil {
		return nil, err
	}
	return &IDaveConsensusConsensusCreationIterator{contract: _IDaveConsensus.contract, event: "ConsensusCreation", logs: logs, sub: sub}, nil
}

// WatchConsensusCreation is a free log subscription operation binding the contract event 0xaf68463e16cb5595a44214bea8d366ecf7cd3410269c50f92c104b50a7829daa.
//
// Solidity: event ConsensusCreation(address inputBox, address appContract, address tournamentFactory)
func (_IDaveConsensus *IDaveConsensusFilterer) WatchConsensusCreation(opts *bind.WatchOpts, sink chan<- *IDaveConsensusConsensusCreation) (event.Subscription, error) {

	logs, sub, err := _IDaveConsensus.contract.WatchLogs(opts, "ConsensusCreation")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IDaveConsensusConsensusCreation)
				if err := _IDaveConsensus.contract.UnpackLog(event, "ConsensusCreation", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseConsensusCreation is a log parse operation binding the contract event 0xaf68463e16cb5595a44214bea8d366ecf7cd3410269c50f92c104b50a7829daa.
//
// Solidity: event ConsensusCreation(address inputBox, address appContract, address tournamentFactory)
func (_IDaveConsensus *IDaveConsensusFilterer) ParseConsensusCreation(log types.Log) (*IDaveConsensusConsensusCreation, error) {
	event := new(IDaveConsensusConsensusCreation)
	if err := _IDaveConsensus.contract.UnpackLog(event, "ConsensusCreation", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IDaveConsensusEpochSealedIterator is returned from FilterEpochSealed and is used to iterate over the raw logs and unpacked data for EpochSealed events raised by the IDaveConsensus contract.
type IDaveConsensusEpochSealedIterator struct {
	Event *IDaveConsensusEpochSealed // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IDaveConsensusEpochSealedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IDaveConsensusEpochSealed)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IDaveConsensusEpochSealed)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IDaveConsensusEpochSealedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IDaveConsensusEpochSealedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IDaveConsensusEpochSealed represents a EpochSealed event raised by the IDaveConsensus contract.
type IDaveConsensusEpochSealed struct {
	EpochNumber             *big.Int
	InputIndexLowerBound    *big.Int
	InputIndexUpperBound    *big.Int
	InitialMachineStateHash [32]byte
	OutputsMerkleRoot       [32]byte
	Tournament              common.Address
	Raw                     types.Log // Blockchain specific contextual infos
}

// FilterEpochSealed is a free log retrieval operation binding the contract event 0xa91d0b68c00a132585cc08007b46ff5f0abc622f5286b5701149b33784764ced.
//
// Solidity: event EpochSealed(uint256 indexed epochNumber, uint256 inputIndexLowerBound, uint256 inputIndexUpperBound, bytes32 initialMachineStateHash, bytes32 outputsMerkleRoot, address tournament)
func (_IDaveConsensus *IDaveConsensusFilterer) FilterEpochSealed(opts *bind.FilterOpts, epochNumber []*big.Int) (*IDaveConsensusEpochSealedIterator, error) {

	var epochNumberRule []interface{}
	for _, epochNumberItem := range epochNumber {
		epochNumberRule = append(epochNumberRule, epochNumberItem)
	}

	logs, sub, err := _IDaveConsensus.contract.FilterLogs(opts, "EpochSealed", epochNumberRule)
	if err != nil {
		return nil, err
	}
	return &IDaveConsensusEpochSealedIterator{contract: _IDaveConsensus.contract, event: "EpochSealed", logs: logs, sub: sub}, nil
}

// WatchEpochSealed is a free log subscription operation binding the contract event 0xa91d0b68c00a132585cc08007b46ff5f0abc622f5286b5701149b33784764ced.
//
// Solidity: event EpochSealed(uint256 indexed epochNumber, uint256 inputIndexLowerBound, uint256 inputIndexUpperBound, bytes32 initialMachineStateHash, bytes32 outputsMerkleRoot, address tournament)
func (_IDaveConsensus *IDaveConsensusFilterer) WatchEpochSealed(opts *bind.WatchOpts, sink chan<- *IDaveConsensusEpochSealed, epochNumber []*big.Int) (event.Subscription, error) {

	var epochNumberRule []interface{}
	for _, epochNumberItem := range epochNumber {
		epochNumberRule = append(epochNumberRule, epochNumberItem)
	}

	logs, sub, err := _IDaveConsensus.contract.WatchLogs(opts, "EpochSealed", epochNumberRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IDaveConsensusEpochSealed)
				if err := _IDaveConsensus.contract.UnpackLog(event, "EpochSealed", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseEpochSealed is a log parse operation binding the contract event 0xa91d0b68c00a132585cc08007b46ff5f0abc622f5286b5701149b33784764ced.
//
// Solidity: event EpochSealed(uint256 indexed epochNumber, uint256 inputIndexLowerBound, uint256 inputIndexUpperBound, bytes32 initialMachineStateHash, bytes32 outputsMerkleRoot, address tournament)
func (_IDaveConsensus *IDaveConsensusFilterer) ParseEpochSealed(log types.Log) (*IDaveConsensusEpochSealed, error) {
	event := new(IDaveConsensusEpochSealed)
	if err := _IDaveConsensus.contract.UnpackLog(event, "EpochSealed", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IDaveConsensusEpochStagedIterator is returned from FilterEpochStaged and is used to iterate over the raw logs and unpacked data for EpochStaged events raised by the IDaveConsensus contract.
type IDaveConsensusEpochStagedIterator struct {
	Event *IDaveConsensusEpochStaged // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IDaveConsensusEpochStagedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IDaveConsensusEpochStaged)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IDaveConsensusEpochStaged)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IDaveConsensusEpochStagedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IDaveConsensusEpochStagedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IDaveConsensusEpochStaged represents a EpochStaged event raised by the IDaveConsensus contract.
type IDaveConsensusEpochStaged struct {
	EpochNumber                      *big.Int
	StagedPostEpochMachineStateHash  [32]byte
	StagedPostEpochOutputsMerkleRoot [32]byte
	Raw                              types.Log // Blockchain specific contextual infos
}

// FilterEpochStaged is a free log retrieval operation binding the contract event 0x13bd4fdfe8d8a96c44e1f8c899cde8f2ae549c60b4768631f1a88541f85bec62.
//
// Solidity: event EpochStaged(uint256 indexed epochNumber, bytes32 stagedPostEpochMachineStateHash, bytes32 stagedPostEpochOutputsMerkleRoot)
func (_IDaveConsensus *IDaveConsensusFilterer) FilterEpochStaged(opts *bind.FilterOpts, epochNumber []*big.Int) (*IDaveConsensusEpochStagedIterator, error) {

	var epochNumberRule []interface{}
	for _, epochNumberItem := range epochNumber {
		epochNumberRule = append(epochNumberRule, epochNumberItem)
	}

	logs, sub, err := _IDaveConsensus.contract.FilterLogs(opts, "EpochStaged", epochNumberRule)
	if err != nil {
		return nil, err
	}
	return &IDaveConsensusEpochStagedIterator{contract: _IDaveConsensus.contract, event: "EpochStaged", logs: logs, sub: sub}, nil
}

// WatchEpochStaged is a free log subscription operation binding the contract event 0x13bd4fdfe8d8a96c44e1f8c899cde8f2ae549c60b4768631f1a88541f85bec62.
//
// Solidity: event EpochStaged(uint256 indexed epochNumber, bytes32 stagedPostEpochMachineStateHash, bytes32 stagedPostEpochOutputsMerkleRoot)
func (_IDaveConsensus *IDaveConsensusFilterer) WatchEpochStaged(opts *bind.WatchOpts, sink chan<- *IDaveConsensusEpochStaged, epochNumber []*big.Int) (event.Subscription, error) {

	var epochNumberRule []interface{}
	for _, epochNumberItem := range epochNumber {
		epochNumberRule = append(epochNumberRule, epochNumberItem)
	}

	logs, sub, err := _IDaveConsensus.contract.WatchLogs(opts, "EpochStaged", epochNumberRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IDaveConsensusEpochStaged)
				if err := _IDaveConsensus.contract.UnpackLog(event, "EpochStaged", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseEpochStaged is a log parse operation binding the contract event 0x13bd4fdfe8d8a96c44e1f8c899cde8f2ae549c60b4768631f1a88541f85bec62.
//
// Solidity: event EpochStaged(uint256 indexed epochNumber, bytes32 stagedPostEpochMachineStateHash, bytes32 stagedPostEpochOutputsMerkleRoot)
func (_IDaveConsensus *IDaveConsensusFilterer) ParseEpochStaged(log types.Log) (*IDaveConsensusEpochStaged, error) {
	event := new(IDaveConsensusEpochStaged)
	if err := _IDaveConsensus.contract.UnpackLog(event, "EpochStaged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IDaveConsensusSentryClaimIterator is returned from FilterSentryClaim and is used to iterate over the raw logs and unpacked data for SentryClaim events raised by the IDaveConsensus contract.
type IDaveConsensusSentryClaimIterator struct {
	Event *IDaveConsensusSentryClaim // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IDaveConsensusSentryClaimIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IDaveConsensusSentryClaim)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IDaveConsensusSentryClaim)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IDaveConsensusSentryClaimIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IDaveConsensusSentryClaimIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IDaveConsensusSentryClaim represents a SentryClaim event raised by the IDaveConsensus contract.
type IDaveConsensusSentryClaim struct {
	EpochNumber               *big.Int
	SentryId                  *big.Int
	Sentry                    common.Address
	PostEpochMachineStateHash [32]byte
	Raw                       types.Log // Blockchain specific contextual infos
}

// FilterSentryClaim is a free log retrieval operation binding the contract event 0x0a242da6706fab1ed52cfaf047d4939b8c7acac1fe8ff75d911758adf345bdda.
//
// Solidity: event SentryClaim(uint256 indexed epochNumber, uint256 indexed sentryId, address indexed sentry, bytes32 postEpochMachineStateHash)
func (_IDaveConsensus *IDaveConsensusFilterer) FilterSentryClaim(opts *bind.FilterOpts, epochNumber []*big.Int, sentryId []*big.Int, sentry []common.Address) (*IDaveConsensusSentryClaimIterator, error) {

	var epochNumberRule []interface{}
	for _, epochNumberItem := range epochNumber {
		epochNumberRule = append(epochNumberRule, epochNumberItem)
	}
	var sentryIdRule []interface{}
	for _, sentryIdItem := range sentryId {
		sentryIdRule = append(sentryIdRule, sentryIdItem)
	}
	var sentryRule []interface{}
	for _, sentryItem := range sentry {
		sentryRule = append(sentryRule, sentryItem)
	}

	logs, sub, err := _IDaveConsensus.contract.FilterLogs(opts, "SentryClaim", epochNumberRule, sentryIdRule, sentryRule)
	if err != nil {
		return nil, err
	}
	return &IDaveConsensusSentryClaimIterator{contract: _IDaveConsensus.contract, event: "SentryClaim", logs: logs, sub: sub}, nil
}

// WatchSentryClaim is a free log subscription operation binding the contract event 0x0a242da6706fab1ed52cfaf047d4939b8c7acac1fe8ff75d911758adf345bdda.
//
// Solidity: event SentryClaim(uint256 indexed epochNumber, uint256 indexed sentryId, address indexed sentry, bytes32 postEpochMachineStateHash)
func (_IDaveConsensus *IDaveConsensusFilterer) WatchSentryClaim(opts *bind.WatchOpts, sink chan<- *IDaveConsensusSentryClaim, epochNumber []*big.Int, sentryId []*big.Int, sentry []common.Address) (event.Subscription, error) {

	var epochNumberRule []interface{}
	for _, epochNumberItem := range epochNumber {
		epochNumberRule = append(epochNumberRule, epochNumberItem)
	}
	var sentryIdRule []interface{}
	for _, sentryIdItem := range sentryId {
		sentryIdRule = append(sentryIdRule, sentryIdItem)
	}
	var sentryRule []interface{}
	for _, sentryItem := range sentry {
		sentryRule = append(sentryRule, sentryItem)
	}

	logs, sub, err := _IDaveConsensus.contract.WatchLogs(opts, "SentryClaim", epochNumberRule, sentryIdRule, sentryRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IDaveConsensusSentryClaim)
				if err := _IDaveConsensus.contract.UnpackLog(event, "SentryClaim", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSentryClaim is a log parse operation binding the contract event 0x0a242da6706fab1ed52cfaf047d4939b8c7acac1fe8ff75d911758adf345bdda.
//
// Solidity: event SentryClaim(uint256 indexed epochNumber, uint256 indexed sentryId, address indexed sentry, bytes32 postEpochMachineStateHash)
func (_IDaveConsensus *IDaveConsensusFilterer) ParseSentryClaim(log types.Log) (*IDaveConsensusSentryClaim, error) {
	event := new(IDaveConsensusSentryClaim)
	if err := _IDaveConsensus.contract.UnpackLog(event, "SentryClaim", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IDaveConsensusSentryRotationIterator is returned from FilterSentryRotation and is used to iterate over the raw logs and unpacked data for SentryRotation events raised by the IDaveConsensus contract.
type IDaveConsensusSentryRotationIterator struct {
	Event *IDaveConsensusSentryRotation // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IDaveConsensusSentryRotationIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IDaveConsensusSentryRotation)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IDaveConsensusSentryRotation)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IDaveConsensusSentryRotationIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IDaveConsensusSentryRotationIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IDaveConsensusSentryRotation represents a SentryRotation event raised by the IDaveConsensus contract.
type IDaveConsensusSentryRotation struct {
	SentryId  *big.Int
	OldSentry common.Address
	NewSentry common.Address
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterSentryRotation is a free log retrieval operation binding the contract event 0x1c5771374c7e30d72546ebada2373852b83c92f2e40da2e8987f059fa732682d.
//
// Solidity: event SentryRotation(uint256 indexed sentryId, address indexed oldSentry, address indexed newSentry)
func (_IDaveConsensus *IDaveConsensusFilterer) FilterSentryRotation(opts *bind.FilterOpts, sentryId []*big.Int, oldSentry []common.Address, newSentry []common.Address) (*IDaveConsensusSentryRotationIterator, error) {

	var sentryIdRule []interface{}
	for _, sentryIdItem := range sentryId {
		sentryIdRule = append(sentryIdRule, sentryIdItem)
	}
	var oldSentryRule []interface{}
	for _, oldSentryItem := range oldSentry {
		oldSentryRule = append(oldSentryRule, oldSentryItem)
	}
	var newSentryRule []interface{}
	for _, newSentryItem := range newSentry {
		newSentryRule = append(newSentryRule, newSentryItem)
	}

	logs, sub, err := _IDaveConsensus.contract.FilterLogs(opts, "SentryRotation", sentryIdRule, oldSentryRule, newSentryRule)
	if err != nil {
		return nil, err
	}
	return &IDaveConsensusSentryRotationIterator{contract: _IDaveConsensus.contract, event: "SentryRotation", logs: logs, sub: sub}, nil
}

// WatchSentryRotation is a free log subscription operation binding the contract event 0x1c5771374c7e30d72546ebada2373852b83c92f2e40da2e8987f059fa732682d.
//
// Solidity: event SentryRotation(uint256 indexed sentryId, address indexed oldSentry, address indexed newSentry)
func (_IDaveConsensus *IDaveConsensusFilterer) WatchSentryRotation(opts *bind.WatchOpts, sink chan<- *IDaveConsensusSentryRotation, sentryId []*big.Int, oldSentry []common.Address, newSentry []common.Address) (event.Subscription, error) {

	var sentryIdRule []interface{}
	for _, sentryIdItem := range sentryId {
		sentryIdRule = append(sentryIdRule, sentryIdItem)
	}
	var oldSentryRule []interface{}
	for _, oldSentryItem := range oldSentry {
		oldSentryRule = append(oldSentryRule, oldSentryItem)
	}
	var newSentryRule []interface{}
	for _, newSentryItem := range newSentry {
		newSentryRule = append(newSentryRule, newSentryItem)
	}

	logs, sub, err := _IDaveConsensus.contract.WatchLogs(opts, "SentryRotation", sentryIdRule, oldSentryRule, newSentryRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IDaveConsensusSentryRotation)
				if err := _IDaveConsensus.contract.UnpackLog(event, "SentryRotation", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSentryRotation is a log parse operation binding the contract event 0x1c5771374c7e30d72546ebada2373852b83c92f2e40da2e8987f059fa732682d.
//
// Solidity: event SentryRotation(uint256 indexed sentryId, address indexed oldSentry, address indexed newSentry)
func (_IDaveConsensus *IDaveConsensusFilterer) ParseSentryRotation(log types.Log) (*IDaveConsensusSentryRotation, error) {
	event := new(IDaveConsensusSentryRotation)
	if err := _IDaveConsensus.contract.UnpackLog(event, "SentryRotation", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
