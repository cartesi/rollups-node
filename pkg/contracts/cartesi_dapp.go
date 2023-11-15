// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package contracts

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

// OutputValidityProof is an auto generated low-level Go binding around an user-defined struct.
type OutputValidityProof struct {
	InputIndexWithinEpoch            uint64
	OutputIndexWithinInput           uint64
	OutputHashesRootHash             [32]byte
	VouchersEpochRootHash            [32]byte
	NoticesEpochRootHash             [32]byte
	MachineStateHash                 [32]byte
	OutputHashInOutputHashesSiblings [][32]byte
	OutputHashesInEpochSiblings      [][32]byte
}

// Proof is an auto generated low-level Go binding around an user-defined struct.
type Proof struct {
	Validity OutputValidityProof
	Context  []byte
}

// CartesiDAppMetaData contains all meta data concerning the CartesiDApp contract.
var CartesiDAppMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"contractIConsensus\",\"name\":\"_consensus\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_owner\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"_templateHash\",\"type\":\"bytes32\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"EtherTransferFailed\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"IncorrectEpochHash\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"IncorrectOutputHashesRootHash\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"IncorrectOutputsEpochRootHash\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InputIndexOutOfClaimBounds\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"OnlyDApp\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"VoucherReexecutionNotAllowed\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"contractIConsensus\",\"name\":\"newConsensus\",\"type\":\"address\"}],\"name\":\"NewConsensus\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"previousOwner\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"OwnershipTransferred\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"voucherId\",\"type\":\"uint256\"}],\"name\":\"VoucherExecuted\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_destination\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"_payload\",\"type\":\"bytes\"},{\"components\":[{\"components\":[{\"internalType\":\"uint64\",\"name\":\"inputIndexWithinEpoch\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"outputIndexWithinInput\",\"type\":\"uint64\"},{\"internalType\":\"bytes32\",\"name\":\"outputHashesRootHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"vouchersEpochRootHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"noticesEpochRootHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"machineStateHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32[]\",\"name\":\"outputHashInOutputHashesSiblings\",\"type\":\"bytes32[]\"},{\"internalType\":\"bytes32[]\",\"name\":\"outputHashesInEpochSiblings\",\"type\":\"bytes32[]\"}],\"internalType\":\"structOutputValidityProof\",\"name\":\"validity\",\"type\":\"tuple\"},{\"internalType\":\"bytes\",\"name\":\"context\",\"type\":\"bytes\"}],\"internalType\":\"structProof\",\"name\":\"_proof\",\"type\":\"tuple\"}],\"name\":\"executeVoucher\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getConsensus\",\"outputs\":[{\"internalType\":\"contractIConsensus\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getTemplateHash\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"contractIConsensus\",\"name\":\"_newConsensus\",\"type\":\"address\"}],\"name\":\"migrateToConsensus\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"uint256[]\",\"name\":\"\",\"type\":\"uint256[]\"},{\"internalType\":\"uint256[]\",\"name\":\"\",\"type\":\"uint256[]\"},{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"}],\"name\":\"onERC1155BatchReceived\",\"outputs\":[{\"internalType\":\"bytes4\",\"name\":\"\",\"type\":\"bytes4\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"}],\"name\":\"onERC1155Received\",\"outputs\":[{\"internalType\":\"bytes4\",\"name\":\"\",\"type\":\"bytes4\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"}],\"name\":\"onERC721Received\",\"outputs\":[{\"internalType\":\"bytes4\",\"name\":\"\",\"type\":\"bytes4\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"renounceOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes4\",\"name\":\"interfaceId\",\"type\":\"bytes4\"}],\"name\":\"supportsInterface\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"transferOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"_notice\",\"type\":\"bytes\"},{\"components\":[{\"components\":[{\"internalType\":\"uint64\",\"name\":\"inputIndexWithinEpoch\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"outputIndexWithinInput\",\"type\":\"uint64\"},{\"internalType\":\"bytes32\",\"name\":\"outputHashesRootHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"vouchersEpochRootHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"noticesEpochRootHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"machineStateHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32[]\",\"name\":\"outputHashInOutputHashesSiblings\",\"type\":\"bytes32[]\"},{\"internalType\":\"bytes32[]\",\"name\":\"outputHashesInEpochSiblings\",\"type\":\"bytes32[]\"}],\"internalType\":\"structOutputValidityProof\",\"name\":\"validity\",\"type\":\"tuple\"},{\"internalType\":\"bytes\",\"name\":\"context\",\"type\":\"bytes\"}],\"internalType\":\"structProof\",\"name\":\"_proof\",\"type\":\"tuple\"}],\"name\":\"validateNotice\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_inputIndex\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"_outputIndexWithinInput\",\"type\":\"uint256\"}],\"name\":\"wasVoucherExecuted\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_receiver\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"_value\",\"type\":\"uint256\"}],\"name\":\"withdrawEther\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"stateMutability\":\"payable\",\"type\":\"receive\"}]",
}

// CartesiDAppABI is the input ABI used to generate the binding from.
// Deprecated: Use CartesiDAppMetaData.ABI instead.
var CartesiDAppABI = CartesiDAppMetaData.ABI

// CartesiDApp is an auto generated Go binding around an Ethereum contract.
type CartesiDApp struct {
	CartesiDAppCaller     // Read-only binding to the contract
	CartesiDAppTransactor // Write-only binding to the contract
	CartesiDAppFilterer   // Log filterer for contract events
}

// CartesiDAppCaller is an auto generated read-only Go binding around an Ethereum contract.
type CartesiDAppCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CartesiDAppTransactor is an auto generated write-only Go binding around an Ethereum contract.
type CartesiDAppTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CartesiDAppFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type CartesiDAppFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CartesiDAppSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type CartesiDAppSession struct {
	Contract     *CartesiDApp      // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// CartesiDAppCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type CartesiDAppCallerSession struct {
	Contract *CartesiDAppCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts      // Call options to use throughout this session
}

// CartesiDAppTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type CartesiDAppTransactorSession struct {
	Contract     *CartesiDAppTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts      // Transaction auth options to use throughout this session
}

// CartesiDAppRaw is an auto generated low-level Go binding around an Ethereum contract.
type CartesiDAppRaw struct {
	Contract *CartesiDApp // Generic contract binding to access the raw methods on
}

// CartesiDAppCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type CartesiDAppCallerRaw struct {
	Contract *CartesiDAppCaller // Generic read-only contract binding to access the raw methods on
}

// CartesiDAppTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type CartesiDAppTransactorRaw struct {
	Contract *CartesiDAppTransactor // Generic write-only contract binding to access the raw methods on
}

// NewCartesiDApp creates a new instance of CartesiDApp, bound to a specific deployed contract.
func NewCartesiDApp(address common.Address, backend bind.ContractBackend) (*CartesiDApp, error) {
	contract, err := bindCartesiDApp(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &CartesiDApp{CartesiDAppCaller: CartesiDAppCaller{contract: contract}, CartesiDAppTransactor: CartesiDAppTransactor{contract: contract}, CartesiDAppFilterer: CartesiDAppFilterer{contract: contract}}, nil
}

// NewCartesiDAppCaller creates a new read-only instance of CartesiDApp, bound to a specific deployed contract.
func NewCartesiDAppCaller(address common.Address, caller bind.ContractCaller) (*CartesiDAppCaller, error) {
	contract, err := bindCartesiDApp(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &CartesiDAppCaller{contract: contract}, nil
}

// NewCartesiDAppTransactor creates a new write-only instance of CartesiDApp, bound to a specific deployed contract.
func NewCartesiDAppTransactor(address common.Address, transactor bind.ContractTransactor) (*CartesiDAppTransactor, error) {
	contract, err := bindCartesiDApp(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &CartesiDAppTransactor{contract: contract}, nil
}

// NewCartesiDAppFilterer creates a new log filterer instance of CartesiDApp, bound to a specific deployed contract.
func NewCartesiDAppFilterer(address common.Address, filterer bind.ContractFilterer) (*CartesiDAppFilterer, error) {
	contract, err := bindCartesiDApp(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &CartesiDAppFilterer{contract: contract}, nil
}

// bindCartesiDApp binds a generic wrapper to an already deployed contract.
func bindCartesiDApp(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := CartesiDAppMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_CartesiDApp *CartesiDAppRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _CartesiDApp.Contract.CartesiDAppCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_CartesiDApp *CartesiDAppRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CartesiDApp.Contract.CartesiDAppTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_CartesiDApp *CartesiDAppRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _CartesiDApp.Contract.CartesiDAppTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_CartesiDApp *CartesiDAppCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _CartesiDApp.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_CartesiDApp *CartesiDAppTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CartesiDApp.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_CartesiDApp *CartesiDAppTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _CartesiDApp.Contract.contract.Transact(opts, method, params...)
}

// GetConsensus is a free data retrieval call binding the contract method 0x179e740b.
//
// Solidity: function getConsensus() view returns(address)
func (_CartesiDApp *CartesiDAppCaller) GetConsensus(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _CartesiDApp.contract.Call(opts, &out, "getConsensus")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetConsensus is a free data retrieval call binding the contract method 0x179e740b.
//
// Solidity: function getConsensus() view returns(address)
func (_CartesiDApp *CartesiDAppSession) GetConsensus() (common.Address, error) {
	return _CartesiDApp.Contract.GetConsensus(&_CartesiDApp.CallOpts)
}

// GetConsensus is a free data retrieval call binding the contract method 0x179e740b.
//
// Solidity: function getConsensus() view returns(address)
func (_CartesiDApp *CartesiDAppCallerSession) GetConsensus() (common.Address, error) {
	return _CartesiDApp.Contract.GetConsensus(&_CartesiDApp.CallOpts)
}

// GetTemplateHash is a free data retrieval call binding the contract method 0x61b12c66.
//
// Solidity: function getTemplateHash() view returns(bytes32)
func (_CartesiDApp *CartesiDAppCaller) GetTemplateHash(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _CartesiDApp.contract.Call(opts, &out, "getTemplateHash")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetTemplateHash is a free data retrieval call binding the contract method 0x61b12c66.
//
// Solidity: function getTemplateHash() view returns(bytes32)
func (_CartesiDApp *CartesiDAppSession) GetTemplateHash() ([32]byte, error) {
	return _CartesiDApp.Contract.GetTemplateHash(&_CartesiDApp.CallOpts)
}

// GetTemplateHash is a free data retrieval call binding the contract method 0x61b12c66.
//
// Solidity: function getTemplateHash() view returns(bytes32)
func (_CartesiDApp *CartesiDAppCallerSession) GetTemplateHash() ([32]byte, error) {
	return _CartesiDApp.Contract.GetTemplateHash(&_CartesiDApp.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_CartesiDApp *CartesiDAppCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _CartesiDApp.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_CartesiDApp *CartesiDAppSession) Owner() (common.Address, error) {
	return _CartesiDApp.Contract.Owner(&_CartesiDApp.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_CartesiDApp *CartesiDAppCallerSession) Owner() (common.Address, error) {
	return _CartesiDApp.Contract.Owner(&_CartesiDApp.CallOpts)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_CartesiDApp *CartesiDAppCaller) SupportsInterface(opts *bind.CallOpts, interfaceId [4]byte) (bool, error) {
	var out []interface{}
	err := _CartesiDApp.contract.Call(opts, &out, "supportsInterface", interfaceId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_CartesiDApp *CartesiDAppSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _CartesiDApp.Contract.SupportsInterface(&_CartesiDApp.CallOpts, interfaceId)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_CartesiDApp *CartesiDAppCallerSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _CartesiDApp.Contract.SupportsInterface(&_CartesiDApp.CallOpts, interfaceId)
}

// ValidateNotice is a free data retrieval call binding the contract method 0x96487d46.
//
// Solidity: function validateNotice(bytes _notice, ((uint64,uint64,bytes32,bytes32,bytes32,bytes32,bytes32[],bytes32[]),bytes) _proof) view returns(bool)
func (_CartesiDApp *CartesiDAppCaller) ValidateNotice(opts *bind.CallOpts, _notice []byte, _proof Proof) (bool, error) {
	var out []interface{}
	err := _CartesiDApp.contract.Call(opts, &out, "validateNotice", _notice, _proof)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// ValidateNotice is a free data retrieval call binding the contract method 0x96487d46.
//
// Solidity: function validateNotice(bytes _notice, ((uint64,uint64,bytes32,bytes32,bytes32,bytes32,bytes32[],bytes32[]),bytes) _proof) view returns(bool)
func (_CartesiDApp *CartesiDAppSession) ValidateNotice(_notice []byte, _proof Proof) (bool, error) {
	return _CartesiDApp.Contract.ValidateNotice(&_CartesiDApp.CallOpts, _notice, _proof)
}

// ValidateNotice is a free data retrieval call binding the contract method 0x96487d46.
//
// Solidity: function validateNotice(bytes _notice, ((uint64,uint64,bytes32,bytes32,bytes32,bytes32,bytes32[],bytes32[]),bytes) _proof) view returns(bool)
func (_CartesiDApp *CartesiDAppCallerSession) ValidateNotice(_notice []byte, _proof Proof) (bool, error) {
	return _CartesiDApp.Contract.ValidateNotice(&_CartesiDApp.CallOpts, _notice, _proof)
}

// WasVoucherExecuted is a free data retrieval call binding the contract method 0x9d9b1145.
//
// Solidity: function wasVoucherExecuted(uint256 _inputIndex, uint256 _outputIndexWithinInput) view returns(bool)
func (_CartesiDApp *CartesiDAppCaller) WasVoucherExecuted(opts *bind.CallOpts, _inputIndex *big.Int, _outputIndexWithinInput *big.Int) (bool, error) {
	var out []interface{}
	err := _CartesiDApp.contract.Call(opts, &out, "wasVoucherExecuted", _inputIndex, _outputIndexWithinInput)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// WasVoucherExecuted is a free data retrieval call binding the contract method 0x9d9b1145.
//
// Solidity: function wasVoucherExecuted(uint256 _inputIndex, uint256 _outputIndexWithinInput) view returns(bool)
func (_CartesiDApp *CartesiDAppSession) WasVoucherExecuted(_inputIndex *big.Int, _outputIndexWithinInput *big.Int) (bool, error) {
	return _CartesiDApp.Contract.WasVoucherExecuted(&_CartesiDApp.CallOpts, _inputIndex, _outputIndexWithinInput)
}

// WasVoucherExecuted is a free data retrieval call binding the contract method 0x9d9b1145.
//
// Solidity: function wasVoucherExecuted(uint256 _inputIndex, uint256 _outputIndexWithinInput) view returns(bool)
func (_CartesiDApp *CartesiDAppCallerSession) WasVoucherExecuted(_inputIndex *big.Int, _outputIndexWithinInput *big.Int) (bool, error) {
	return _CartesiDApp.Contract.WasVoucherExecuted(&_CartesiDApp.CallOpts, _inputIndex, _outputIndexWithinInput)
}

// ExecuteVoucher is a paid mutator transaction binding the contract method 0x1250482f.
//
// Solidity: function executeVoucher(address _destination, bytes _payload, ((uint64,uint64,bytes32,bytes32,bytes32,bytes32,bytes32[],bytes32[]),bytes) _proof) returns(bool)
func (_CartesiDApp *CartesiDAppTransactor) ExecuteVoucher(opts *bind.TransactOpts, _destination common.Address, _payload []byte, _proof Proof) (*types.Transaction, error) {
	return _CartesiDApp.contract.Transact(opts, "executeVoucher", _destination, _payload, _proof)
}

// ExecuteVoucher is a paid mutator transaction binding the contract method 0x1250482f.
//
// Solidity: function executeVoucher(address _destination, bytes _payload, ((uint64,uint64,bytes32,bytes32,bytes32,bytes32,bytes32[],bytes32[]),bytes) _proof) returns(bool)
func (_CartesiDApp *CartesiDAppSession) ExecuteVoucher(_destination common.Address, _payload []byte, _proof Proof) (*types.Transaction, error) {
	return _CartesiDApp.Contract.ExecuteVoucher(&_CartesiDApp.TransactOpts, _destination, _payload, _proof)
}

// ExecuteVoucher is a paid mutator transaction binding the contract method 0x1250482f.
//
// Solidity: function executeVoucher(address _destination, bytes _payload, ((uint64,uint64,bytes32,bytes32,bytes32,bytes32,bytes32[],bytes32[]),bytes) _proof) returns(bool)
func (_CartesiDApp *CartesiDAppTransactorSession) ExecuteVoucher(_destination common.Address, _payload []byte, _proof Proof) (*types.Transaction, error) {
	return _CartesiDApp.Contract.ExecuteVoucher(&_CartesiDApp.TransactOpts, _destination, _payload, _proof)
}

// MigrateToConsensus is a paid mutator transaction binding the contract method 0xfc411683.
//
// Solidity: function migrateToConsensus(address _newConsensus) returns()
func (_CartesiDApp *CartesiDAppTransactor) MigrateToConsensus(opts *bind.TransactOpts, _newConsensus common.Address) (*types.Transaction, error) {
	return _CartesiDApp.contract.Transact(opts, "migrateToConsensus", _newConsensus)
}

// MigrateToConsensus is a paid mutator transaction binding the contract method 0xfc411683.
//
// Solidity: function migrateToConsensus(address _newConsensus) returns()
func (_CartesiDApp *CartesiDAppSession) MigrateToConsensus(_newConsensus common.Address) (*types.Transaction, error) {
	return _CartesiDApp.Contract.MigrateToConsensus(&_CartesiDApp.TransactOpts, _newConsensus)
}

// MigrateToConsensus is a paid mutator transaction binding the contract method 0xfc411683.
//
// Solidity: function migrateToConsensus(address _newConsensus) returns()
func (_CartesiDApp *CartesiDAppTransactorSession) MigrateToConsensus(_newConsensus common.Address) (*types.Transaction, error) {
	return _CartesiDApp.Contract.MigrateToConsensus(&_CartesiDApp.TransactOpts, _newConsensus)
}

// OnERC1155BatchReceived is a paid mutator transaction binding the contract method 0xbc197c81.
//
// Solidity: function onERC1155BatchReceived(address , address , uint256[] , uint256[] , bytes ) returns(bytes4)
func (_CartesiDApp *CartesiDAppTransactor) OnERC1155BatchReceived(opts *bind.TransactOpts, arg0 common.Address, arg1 common.Address, arg2 []*big.Int, arg3 []*big.Int, arg4 []byte) (*types.Transaction, error) {
	return _CartesiDApp.contract.Transact(opts, "onERC1155BatchReceived", arg0, arg1, arg2, arg3, arg4)
}

// OnERC1155BatchReceived is a paid mutator transaction binding the contract method 0xbc197c81.
//
// Solidity: function onERC1155BatchReceived(address , address , uint256[] , uint256[] , bytes ) returns(bytes4)
func (_CartesiDApp *CartesiDAppSession) OnERC1155BatchReceived(arg0 common.Address, arg1 common.Address, arg2 []*big.Int, arg3 []*big.Int, arg4 []byte) (*types.Transaction, error) {
	return _CartesiDApp.Contract.OnERC1155BatchReceived(&_CartesiDApp.TransactOpts, arg0, arg1, arg2, arg3, arg4)
}

// OnERC1155BatchReceived is a paid mutator transaction binding the contract method 0xbc197c81.
//
// Solidity: function onERC1155BatchReceived(address , address , uint256[] , uint256[] , bytes ) returns(bytes4)
func (_CartesiDApp *CartesiDAppTransactorSession) OnERC1155BatchReceived(arg0 common.Address, arg1 common.Address, arg2 []*big.Int, arg3 []*big.Int, arg4 []byte) (*types.Transaction, error) {
	return _CartesiDApp.Contract.OnERC1155BatchReceived(&_CartesiDApp.TransactOpts, arg0, arg1, arg2, arg3, arg4)
}

// OnERC1155Received is a paid mutator transaction binding the contract method 0xf23a6e61.
//
// Solidity: function onERC1155Received(address , address , uint256 , uint256 , bytes ) returns(bytes4)
func (_CartesiDApp *CartesiDAppTransactor) OnERC1155Received(opts *bind.TransactOpts, arg0 common.Address, arg1 common.Address, arg2 *big.Int, arg3 *big.Int, arg4 []byte) (*types.Transaction, error) {
	return _CartesiDApp.contract.Transact(opts, "onERC1155Received", arg0, arg1, arg2, arg3, arg4)
}

// OnERC1155Received is a paid mutator transaction binding the contract method 0xf23a6e61.
//
// Solidity: function onERC1155Received(address , address , uint256 , uint256 , bytes ) returns(bytes4)
func (_CartesiDApp *CartesiDAppSession) OnERC1155Received(arg0 common.Address, arg1 common.Address, arg2 *big.Int, arg3 *big.Int, arg4 []byte) (*types.Transaction, error) {
	return _CartesiDApp.Contract.OnERC1155Received(&_CartesiDApp.TransactOpts, arg0, arg1, arg2, arg3, arg4)
}

// OnERC1155Received is a paid mutator transaction binding the contract method 0xf23a6e61.
//
// Solidity: function onERC1155Received(address , address , uint256 , uint256 , bytes ) returns(bytes4)
func (_CartesiDApp *CartesiDAppTransactorSession) OnERC1155Received(arg0 common.Address, arg1 common.Address, arg2 *big.Int, arg3 *big.Int, arg4 []byte) (*types.Transaction, error) {
	return _CartesiDApp.Contract.OnERC1155Received(&_CartesiDApp.TransactOpts, arg0, arg1, arg2, arg3, arg4)
}

// OnERC721Received is a paid mutator transaction binding the contract method 0x150b7a02.
//
// Solidity: function onERC721Received(address , address , uint256 , bytes ) returns(bytes4)
func (_CartesiDApp *CartesiDAppTransactor) OnERC721Received(opts *bind.TransactOpts, arg0 common.Address, arg1 common.Address, arg2 *big.Int, arg3 []byte) (*types.Transaction, error) {
	return _CartesiDApp.contract.Transact(opts, "onERC721Received", arg0, arg1, arg2, arg3)
}

// OnERC721Received is a paid mutator transaction binding the contract method 0x150b7a02.
//
// Solidity: function onERC721Received(address , address , uint256 , bytes ) returns(bytes4)
func (_CartesiDApp *CartesiDAppSession) OnERC721Received(arg0 common.Address, arg1 common.Address, arg2 *big.Int, arg3 []byte) (*types.Transaction, error) {
	return _CartesiDApp.Contract.OnERC721Received(&_CartesiDApp.TransactOpts, arg0, arg1, arg2, arg3)
}

// OnERC721Received is a paid mutator transaction binding the contract method 0x150b7a02.
//
// Solidity: function onERC721Received(address , address , uint256 , bytes ) returns(bytes4)
func (_CartesiDApp *CartesiDAppTransactorSession) OnERC721Received(arg0 common.Address, arg1 common.Address, arg2 *big.Int, arg3 []byte) (*types.Transaction, error) {
	return _CartesiDApp.Contract.OnERC721Received(&_CartesiDApp.TransactOpts, arg0, arg1, arg2, arg3)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_CartesiDApp *CartesiDAppTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CartesiDApp.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_CartesiDApp *CartesiDAppSession) RenounceOwnership() (*types.Transaction, error) {
	return _CartesiDApp.Contract.RenounceOwnership(&_CartesiDApp.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_CartesiDApp *CartesiDAppTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _CartesiDApp.Contract.RenounceOwnership(&_CartesiDApp.TransactOpts)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_CartesiDApp *CartesiDAppTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _CartesiDApp.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_CartesiDApp *CartesiDAppSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _CartesiDApp.Contract.TransferOwnership(&_CartesiDApp.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_CartesiDApp *CartesiDAppTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _CartesiDApp.Contract.TransferOwnership(&_CartesiDApp.TransactOpts, newOwner)
}

// WithdrawEther is a paid mutator transaction binding the contract method 0x522f6815.
//
// Solidity: function withdrawEther(address _receiver, uint256 _value) returns()
func (_CartesiDApp *CartesiDAppTransactor) WithdrawEther(opts *bind.TransactOpts, _receiver common.Address, _value *big.Int) (*types.Transaction, error) {
	return _CartesiDApp.contract.Transact(opts, "withdrawEther", _receiver, _value)
}

// WithdrawEther is a paid mutator transaction binding the contract method 0x522f6815.
//
// Solidity: function withdrawEther(address _receiver, uint256 _value) returns()
func (_CartesiDApp *CartesiDAppSession) WithdrawEther(_receiver common.Address, _value *big.Int) (*types.Transaction, error) {
	return _CartesiDApp.Contract.WithdrawEther(&_CartesiDApp.TransactOpts, _receiver, _value)
}

// WithdrawEther is a paid mutator transaction binding the contract method 0x522f6815.
//
// Solidity: function withdrawEther(address _receiver, uint256 _value) returns()
func (_CartesiDApp *CartesiDAppTransactorSession) WithdrawEther(_receiver common.Address, _value *big.Int) (*types.Transaction, error) {
	return _CartesiDApp.Contract.WithdrawEther(&_CartesiDApp.TransactOpts, _receiver, _value)
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_CartesiDApp *CartesiDAppTransactor) Receive(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CartesiDApp.contract.RawTransact(opts, nil) // calldata is disallowed for receive function
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_CartesiDApp *CartesiDAppSession) Receive() (*types.Transaction, error) {
	return _CartesiDApp.Contract.Receive(&_CartesiDApp.TransactOpts)
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_CartesiDApp *CartesiDAppTransactorSession) Receive() (*types.Transaction, error) {
	return _CartesiDApp.Contract.Receive(&_CartesiDApp.TransactOpts)
}

// CartesiDAppNewConsensusIterator is returned from FilterNewConsensus and is used to iterate over the raw logs and unpacked data for NewConsensus events raised by the CartesiDApp contract.
type CartesiDAppNewConsensusIterator struct {
	Event *CartesiDAppNewConsensus // Event containing the contract specifics and raw log

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
func (it *CartesiDAppNewConsensusIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(CartesiDAppNewConsensus)
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
		it.Event = new(CartesiDAppNewConsensus)
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
func (it *CartesiDAppNewConsensusIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *CartesiDAppNewConsensusIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// CartesiDAppNewConsensus represents a NewConsensus event raised by the CartesiDApp contract.
type CartesiDAppNewConsensus struct {
	NewConsensus common.Address
	Raw          types.Log // Blockchain specific contextual infos
}

// FilterNewConsensus is a free log retrieval operation binding the contract event 0x4991c6f37185659e276ff918a96f3e20e6c5abcd8c9aab450dc19c2f7ad35cb5.
//
// Solidity: event NewConsensus(address newConsensus)
func (_CartesiDApp *CartesiDAppFilterer) FilterNewConsensus(opts *bind.FilterOpts) (*CartesiDAppNewConsensusIterator, error) {

	logs, sub, err := _CartesiDApp.contract.FilterLogs(opts, "NewConsensus")
	if err != nil {
		return nil, err
	}
	return &CartesiDAppNewConsensusIterator{contract: _CartesiDApp.contract, event: "NewConsensus", logs: logs, sub: sub}, nil
}

// WatchNewConsensus is a free log subscription operation binding the contract event 0x4991c6f37185659e276ff918a96f3e20e6c5abcd8c9aab450dc19c2f7ad35cb5.
//
// Solidity: event NewConsensus(address newConsensus)
func (_CartesiDApp *CartesiDAppFilterer) WatchNewConsensus(opts *bind.WatchOpts, sink chan<- *CartesiDAppNewConsensus) (event.Subscription, error) {

	logs, sub, err := _CartesiDApp.contract.WatchLogs(opts, "NewConsensus")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(CartesiDAppNewConsensus)
				if err := _CartesiDApp.contract.UnpackLog(event, "NewConsensus", log); err != nil {
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

// ParseNewConsensus is a log parse operation binding the contract event 0x4991c6f37185659e276ff918a96f3e20e6c5abcd8c9aab450dc19c2f7ad35cb5.
//
// Solidity: event NewConsensus(address newConsensus)
func (_CartesiDApp *CartesiDAppFilterer) ParseNewConsensus(log types.Log) (*CartesiDAppNewConsensus, error) {
	event := new(CartesiDAppNewConsensus)
	if err := _CartesiDApp.contract.UnpackLog(event, "NewConsensus", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// CartesiDAppOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the CartesiDApp contract.
type CartesiDAppOwnershipTransferredIterator struct {
	Event *CartesiDAppOwnershipTransferred // Event containing the contract specifics and raw log

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
func (it *CartesiDAppOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(CartesiDAppOwnershipTransferred)
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
		it.Event = new(CartesiDAppOwnershipTransferred)
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
func (it *CartesiDAppOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *CartesiDAppOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// CartesiDAppOwnershipTransferred represents a OwnershipTransferred event raised by the CartesiDApp contract.
type CartesiDAppOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_CartesiDApp *CartesiDAppFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*CartesiDAppOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _CartesiDApp.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &CartesiDAppOwnershipTransferredIterator{contract: _CartesiDApp.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_CartesiDApp *CartesiDAppFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *CartesiDAppOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _CartesiDApp.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(CartesiDAppOwnershipTransferred)
				if err := _CartesiDApp.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
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

// ParseOwnershipTransferred is a log parse operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_CartesiDApp *CartesiDAppFilterer) ParseOwnershipTransferred(log types.Log) (*CartesiDAppOwnershipTransferred, error) {
	event := new(CartesiDAppOwnershipTransferred)
	if err := _CartesiDApp.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// CartesiDAppVoucherExecutedIterator is returned from FilterVoucherExecuted and is used to iterate over the raw logs and unpacked data for VoucherExecuted events raised by the CartesiDApp contract.
type CartesiDAppVoucherExecutedIterator struct {
	Event *CartesiDAppVoucherExecuted // Event containing the contract specifics and raw log

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
func (it *CartesiDAppVoucherExecutedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(CartesiDAppVoucherExecuted)
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
		it.Event = new(CartesiDAppVoucherExecuted)
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
func (it *CartesiDAppVoucherExecutedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *CartesiDAppVoucherExecutedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// CartesiDAppVoucherExecuted represents a VoucherExecuted event raised by the CartesiDApp contract.
type CartesiDAppVoucherExecuted struct {
	VoucherId *big.Int
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterVoucherExecuted is a free log retrieval operation binding the contract event 0x0eb7ee080f865f1cadc4f54daf58cc3b8879e888832867d13351edcec0fbdc54.
//
// Solidity: event VoucherExecuted(uint256 voucherId)
func (_CartesiDApp *CartesiDAppFilterer) FilterVoucherExecuted(opts *bind.FilterOpts) (*CartesiDAppVoucherExecutedIterator, error) {

	logs, sub, err := _CartesiDApp.contract.FilterLogs(opts, "VoucherExecuted")
	if err != nil {
		return nil, err
	}
	return &CartesiDAppVoucherExecutedIterator{contract: _CartesiDApp.contract, event: "VoucherExecuted", logs: logs, sub: sub}, nil
}

// WatchVoucherExecuted is a free log subscription operation binding the contract event 0x0eb7ee080f865f1cadc4f54daf58cc3b8879e888832867d13351edcec0fbdc54.
//
// Solidity: event VoucherExecuted(uint256 voucherId)
func (_CartesiDApp *CartesiDAppFilterer) WatchVoucherExecuted(opts *bind.WatchOpts, sink chan<- *CartesiDAppVoucherExecuted) (event.Subscription, error) {

	logs, sub, err := _CartesiDApp.contract.WatchLogs(opts, "VoucherExecuted")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(CartesiDAppVoucherExecuted)
				if err := _CartesiDApp.contract.UnpackLog(event, "VoucherExecuted", log); err != nil {
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

// ParseVoucherExecuted is a log parse operation binding the contract event 0x0eb7ee080f865f1cadc4f54daf58cc3b8879e888832867d13351edcec0fbdc54.
//
// Solidity: event VoucherExecuted(uint256 voucherId)
func (_CartesiDApp *CartesiDAppFilterer) ParseVoucherExecuted(log types.Log) (*CartesiDAppVoucherExecuted, error) {
	event := new(CartesiDAppVoucherExecuted)
	if err := _CartesiDApp.contract.UnpackLog(event, "VoucherExecuted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
