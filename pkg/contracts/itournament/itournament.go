// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package itournament

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

// ITournamentBisectingMatchView is an auto generated low-level Go binding around an user-defined struct.
type ITournamentBisectingMatchView struct {
	RevealingParent      [32]byte
	WaitingLeft          [32]byte
	WaitingRight         [32]byte
	SegmentStartPosition *big.Int
	SegmentStartCycle    *big.Int
	CurrentHeight        uint64
	Responder            uint8
}

// ITournamentCommitmentStandingView is an auto generated low-level Go binding around an user-defined struct.
type ITournamentCommitmentStandingView struct {
	Joined         bool
	FinalState     [32]byte
	Claimer        common.Address
	ClockRunning   bool
	ClockDeadline  uint64
	ClockAllowance uint64
}

// ITournamentInnerResultView is an auto generated low-level Go binding around an user-defined struct.
type ITournamentInnerResultView struct {
	Disposition      uint8
	ParentCommitment [32]byte
	PausedAllowance  uint64
}

// ITournamentReadyToSealMatchView is an auto generated low-level Go binding around an user-defined struct.
type ITournamentReadyToSealMatchView struct {
	RevealingParent      [32]byte
	WaitingLeft          [32]byte
	WaitingRight         [32]byte
	SegmentStartPosition *big.Int
	SegmentStartCycle    *big.Int
	Responder            uint8
}

// ITournamentSealedMatchView is an auto generated low-level Go binding around an user-defined struct.
type ITournamentSealedMatchView struct {
	AgreeState         [32]byte
	DivergencePosition *big.Int
	DivergenceCycle    *big.Int
	FinalStateOne      [32]byte
	FinalStateTwo      [32]byte
}

// ITournamentTournamentDescriptor is an auto generated low-level Go binding around an user-defined struct.
type ITournamentTournamentDescriptor struct {
	InitialHash  [32]byte
	BaseCycle    *big.Int
	Log2Stride   uint64
	Height       uint64
	Level        uint64
	Kind         uint8
	StartInstant uint64
	Allowance    uint64
}

// ITournamentTournamentStandingView is an auto generated low-level Go binding around an user-defined struct.
type ITournamentTournamentStandingView struct {
	Standing         uint8
	AcceptsJoins     bool
	HasCandidate     bool
	Candidate        [32]byte
	FinalState       [32]byte
	ParentCommitment [32]byte
	FinishedAt       uint64
	WinnerExpiresAt  uint64
}

// MatchId is an auto generated low-level Go binding around an user-defined struct.
type MatchId struct {
	CommitmentOne [32]byte
	CommitmentTwo [32]byte
}

// ITournamentMetaData contains all meta data concerning the ITournament contract.
var ITournamentMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"advanceMatch\",\"inputs\":[{\"name\":\"matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"newLeftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"newRightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"bisectingMatch\",\"inputs\":[{\"name\":\"matchIdHash\",\"type\":\"bytes32\",\"internalType\":\"Match.IdHash\"}],\"outputs\":[{\"name\":\"actualPhase\",\"type\":\"uint8\",\"internalType\":\"enumMatch.Phase\"},{\"name\":\"value\",\"type\":\"tuple\",\"internalType\":\"structITournament.BisectingMatchView\",\"components\":[{\"name\":\"revealingParent\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"waitingLeft\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"waitingRight\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"segmentStartPosition\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"segmentStartCycle\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentHeight\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"responder\",\"type\":\"uint8\",\"internalType\":\"enumITournament.CommitmentSide\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"bondRecovery\",\"inputs\":[],\"outputs\":[{\"name\":\"disposition\",\"type\":\"uint8\",\"internalType\":\"enumITournament.BondDisposition\"},{\"name\":\"claimer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"payment\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"bondValue\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"classifyMatchTimeout\",\"inputs\":[{\"name\":\"matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}],\"outputs\":[{\"name\":\"actualPhase\",\"type\":\"uint8\",\"internalType\":\"enumMatch.Phase\"},{\"name\":\"outcome\",\"type\":\"uint8\",\"internalType\":\"enumITournament.MatchTimeoutOutcome\"},{\"name\":\"deferredCharge\",\"type\":\"uint64\",\"internalType\":\"Time.Duration\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"commitmentStanding\",\"inputs\":[{\"name\":\"commitmentRoot\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structITournament.CommitmentStandingView\",\"components\":[{\"name\":\"joined\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"finalState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"claimer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"clockRunning\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"clockDeadline\",\"type\":\"uint64\",\"internalType\":\"Time.Instant\"},{\"name\":\"clockAllowance\",\"type\":\"uint64\",\"internalType\":\"Time.Duration\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"eliminateInnerTournament\",\"inputs\":[{\"name\":\"childTournament\",\"type\":\"address\",\"internalType\":\"contractITournament\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"eliminateMatchByTimeout\",\"inputs\":[{\"name\":\"matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getCommitmentJoinedCount\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getLeafMatchSealedCount\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMatchAdvancedCount\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMatchCreatedCount\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMatchDeletedCount\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNewInnerTournamentCount\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"innerResult\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structITournament.InnerResultView\",\"components\":[{\"name\":\"disposition\",\"type\":\"uint8\",\"internalType\":\"enumITournament.InnerTournamentDisposition\"},{\"name\":\"parentCommitment\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"pausedAllowance\",\"type\":\"uint64\",\"internalType\":\"Time.Duration\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"joinTournament\",\"inputs\":[{\"name\":\"finalState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"proof\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"},{\"name\":\"leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"payable\"},{\"type\":\"function\",\"name\":\"readyToSealMatch\",\"inputs\":[{\"name\":\"matchIdHash\",\"type\":\"bytes32\",\"internalType\":\"Match.IdHash\"}],\"outputs\":[{\"name\":\"actualPhase\",\"type\":\"uint8\",\"internalType\":\"enumMatch.Phase\"},{\"name\":\"value\",\"type\":\"tuple\",\"internalType\":\"structITournament.ReadyToSealMatchView\",\"components\":[{\"name\":\"revealingParent\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"waitingLeft\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"waitingRight\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"segmentStartPosition\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"segmentStartCycle\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"responder\",\"type\":\"uint8\",\"internalType\":\"enumITournament.CommitmentSide\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"sealInnerMatchAndCreateInnerTournament\",\"inputs\":[{\"name\":\"matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"leftLeaf\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"rightLeaf\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"agreeHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"agreeHashProof\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"sealLeafMatch\",\"inputs\":[{\"name\":\"matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"leftLeaf\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"rightLeaf\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"agreeHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"agreeHashProof\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"sealedMatch\",\"inputs\":[{\"name\":\"matchIdHash\",\"type\":\"bytes32\",\"internalType\":\"Match.IdHash\"}],\"outputs\":[{\"name\":\"actualPhase\",\"type\":\"uint8\",\"internalType\":\"enumMatch.Phase\"},{\"name\":\"value\",\"type\":\"tuple\",\"internalType\":\"structITournament.SealedMatchView\",\"components\":[{\"name\":\"agreeState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"divergencePosition\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"divergenceCycle\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"finalStateOne\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"finalStateTwo\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"tournamentDescriptor\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structITournament.TournamentDescriptor\",\"components\":[{\"name\":\"initialHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"baseCycle\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"log2Stride\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"height\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"level\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"kind\",\"type\":\"uint8\",\"internalType\":\"enumITournament.TournamentKind\"},{\"name\":\"startInstant\",\"type\":\"uint64\",\"internalType\":\"Time.Instant\"},{\"name\":\"allowance\",\"type\":\"uint64\",\"internalType\":\"Time.Duration\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"tournamentStanding\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structITournament.TournamentStandingView\",\"components\":[{\"name\":\"standing\",\"type\":\"uint8\",\"internalType\":\"enumITournament.TournamentStanding\"},{\"name\":\"acceptsJoins\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"hasCandidate\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"candidate\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"finalState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"parentCommitment\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"finishedAt\",\"type\":\"uint64\",\"internalType\":\"Time.Instant\"},{\"name\":\"winnerExpiresAt\",\"type\":\"uint64\",\"internalType\":\"Time.Instant\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"tryRecoveringBond\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"winInnerTournament\",\"inputs\":[{\"name\":\"childTournament\",\"type\":\"address\",\"internalType\":\"contractITournament\"},{\"name\":\"leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"winLeafMatch\",\"inputs\":[{\"name\":\"matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"proofs\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"winMatchByTimeout\",\"inputs\":[{\"name\":\"matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"BondRecovered\",\"inputs\":[{\"name\":\"commitment\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"claimer\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"payment\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"burned\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"CommitmentJoined\",\"inputs\":[{\"name\":\"commitment\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"finalStateHash\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Machine.Hash\"},{\"name\":\"submitter\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"LeafMatchSealed\",\"inputs\":[{\"name\":\"matchIdHash\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Match.IdHash\"},{\"name\":\"eliminableAt\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"Time.Instant\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"MatchAdvanced\",\"inputs\":[{\"name\":\"matchIdHash\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Match.IdHash\"},{\"name\":\"otherParent\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"},{\"name\":\"leftNode\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"},{\"name\":\"segmentStartPosition\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"eliminableAt\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"Time.Instant\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"MatchCreated\",\"inputs\":[{\"name\":\"matchIdHash\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Match.IdHash\"},{\"name\":\"one\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"two\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"leftOfTwo\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"},{\"name\":\"eliminableAt\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"Time.Instant\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"MatchDeleted\",\"inputs\":[{\"name\":\"matchIdHash\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Match.IdHash\"},{\"name\":\"one\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"two\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"reason\",\"type\":\"uint8\",\"indexed\":false,\"internalType\":\"enumITournament.MatchDeletionReason\"},{\"name\":\"winnerCommitment\",\"type\":\"uint8\",\"indexed\":false,\"internalType\":\"enumITournament.WinnerCommitment\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"NewInnerTournament\",\"inputs\":[{\"name\":\"matchIdHash\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Match.IdHash\"},{\"name\":\"childTournament\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"contractITournament\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"PartialBondRefund\",\"inputs\":[{\"name\":\"recipient\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"value\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"success\",\"type\":\"bool\",\"indexed\":true,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"CannotAdvanceTimedOutClock\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ChildTournamentCannotBeEliminated\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ChildTournamentMustBeEliminated\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ChildTournamentNotFinished\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ClockAlreadyInitialized\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"CommitmentProofWrongSize\",\"inputs\":[{\"name\":\"treeHeight\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"siblingsLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"CommitmentStateMismatch\",\"inputs\":[{\"name\":\"expected\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"computed\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"type\":\"error\",\"name\":\"IncorrectAgreeState\",\"inputs\":[{\"name\":\"initialState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"agreeState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}]},{\"type\":\"error\",\"name\":\"InsufficientBond\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidChildrenNodes\",\"inputs\":[{\"name\":\"expectedParent\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"leftChild\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"rightChild\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"type\":\"error\",\"name\":\"InvalidContestedFinalState\",\"inputs\":[{\"name\":\"contestedFinalStateOne\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"contestedFinalStateTwo\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"finalState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}]},{\"type\":\"error\",\"name\":\"MatchCannotBeAdvanced\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"MatchCannotBeEliminatedByTimeout\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"MatchCannotBeSealed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"MatchCannotBeWonByTimeout\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"MatchDoesNotExist\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"MatchIsNotSealed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NoWinner\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ReentrancyDetected\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"RequireLeafTournament\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"RequireNonLeafTournament\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"RequireNonRootTournament\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TournamentFailedNoWinner\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TournamentIsClosed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TournamentIsFinished\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TournamentNotFinished\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"WrongChildren\",\"inputs\":[{\"name\":\"whichCommitment\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"commitmentRoot\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"left\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"right\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"type\":\"error\",\"name\":\"WrongFinalState\",\"inputs\":[{\"name\":\"whichCommitment\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"computedPostState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"committedPostState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}]},{\"type\":\"error\",\"name\":\"WrongNodesForStep\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"WrongTournamentWinner\",\"inputs\":[{\"name\":\"commitmentRoot\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"winner\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}]",
}

// ITournamentABI is the input ABI used to generate the binding from.
// Deprecated: Use ITournamentMetaData.ABI instead.
var ITournamentABI = ITournamentMetaData.ABI

// ITournament is an auto generated Go binding around an Ethereum contract.
type ITournament struct {
	ITournamentCaller     // Read-only binding to the contract
	ITournamentTransactor // Write-only binding to the contract
	ITournamentFilterer   // Log filterer for contract events
}

// ITournamentCaller is an auto generated read-only Go binding around an Ethereum contract.
type ITournamentCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ITournamentTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ITournamentTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ITournamentFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ITournamentFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ITournamentSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ITournamentSession struct {
	Contract     *ITournament      // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// ITournamentCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ITournamentCallerSession struct {
	Contract *ITournamentCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts      // Call options to use throughout this session
}

// ITournamentTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ITournamentTransactorSession struct {
	Contract     *ITournamentTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts      // Transaction auth options to use throughout this session
}

// ITournamentRaw is an auto generated low-level Go binding around an Ethereum contract.
type ITournamentRaw struct {
	Contract *ITournament // Generic contract binding to access the raw methods on
}

// ITournamentCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ITournamentCallerRaw struct {
	Contract *ITournamentCaller // Generic read-only contract binding to access the raw methods on
}

// ITournamentTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ITournamentTransactorRaw struct {
	Contract *ITournamentTransactor // Generic write-only contract binding to access the raw methods on
}

// NewITournament creates a new instance of ITournament, bound to a specific deployed contract.
func NewITournament(address common.Address, backend bind.ContractBackend) (*ITournament, error) {
	contract, err := bindITournament(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &ITournament{ITournamentCaller: ITournamentCaller{contract: contract}, ITournamentTransactor: ITournamentTransactor{contract: contract}, ITournamentFilterer: ITournamentFilterer{contract: contract}}, nil
}

// NewITournamentCaller creates a new read-only instance of ITournament, bound to a specific deployed contract.
func NewITournamentCaller(address common.Address, caller bind.ContractCaller) (*ITournamentCaller, error) {
	contract, err := bindITournament(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ITournamentCaller{contract: contract}, nil
}

// NewITournamentTransactor creates a new write-only instance of ITournament, bound to a specific deployed contract.
func NewITournamentTransactor(address common.Address, transactor bind.ContractTransactor) (*ITournamentTransactor, error) {
	contract, err := bindITournament(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ITournamentTransactor{contract: contract}, nil
}

// NewITournamentFilterer creates a new log filterer instance of ITournament, bound to a specific deployed contract.
func NewITournamentFilterer(address common.Address, filterer bind.ContractFilterer) (*ITournamentFilterer, error) {
	contract, err := bindITournament(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ITournamentFilterer{contract: contract}, nil
}

// bindITournament binds a generic wrapper to an already deployed contract.
func bindITournament(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := ITournamentMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ITournament *ITournamentRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ITournament.Contract.ITournamentCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ITournament *ITournamentRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ITournament.Contract.ITournamentTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ITournament *ITournamentRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ITournament.Contract.ITournamentTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ITournament *ITournamentCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ITournament.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ITournament *ITournamentTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ITournament.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ITournament *ITournamentTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ITournament.Contract.contract.Transact(opts, method, params...)
}

// BisectingMatch is a free data retrieval call binding the contract method 0xb8bc3e5d.
//
// Solidity: function bisectingMatch(bytes32 matchIdHash) view returns(uint8 actualPhase, (bytes32,bytes32,bytes32,uint256,uint256,uint64,uint8) value)
func (_ITournament *ITournamentCaller) BisectingMatch(opts *bind.CallOpts, matchIdHash [32]byte) (struct {
	ActualPhase uint8
	Value       ITournamentBisectingMatchView
}, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "bisectingMatch", matchIdHash)

	outstruct := new(struct {
		ActualPhase uint8
		Value       ITournamentBisectingMatchView
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.ActualPhase = *abi.ConvertType(out[0], new(uint8)).(*uint8)
	outstruct.Value = *abi.ConvertType(out[1], new(ITournamentBisectingMatchView)).(*ITournamentBisectingMatchView)

	return *outstruct, err

}

// BisectingMatch is a free data retrieval call binding the contract method 0xb8bc3e5d.
//
// Solidity: function bisectingMatch(bytes32 matchIdHash) view returns(uint8 actualPhase, (bytes32,bytes32,bytes32,uint256,uint256,uint64,uint8) value)
func (_ITournament *ITournamentSession) BisectingMatch(matchIdHash [32]byte) (struct {
	ActualPhase uint8
	Value       ITournamentBisectingMatchView
}, error) {
	return _ITournament.Contract.BisectingMatch(&_ITournament.CallOpts, matchIdHash)
}

// BisectingMatch is a free data retrieval call binding the contract method 0xb8bc3e5d.
//
// Solidity: function bisectingMatch(bytes32 matchIdHash) view returns(uint8 actualPhase, (bytes32,bytes32,bytes32,uint256,uint256,uint64,uint8) value)
func (_ITournament *ITournamentCallerSession) BisectingMatch(matchIdHash [32]byte) (struct {
	ActualPhase uint8
	Value       ITournamentBisectingMatchView
}, error) {
	return _ITournament.Contract.BisectingMatch(&_ITournament.CallOpts, matchIdHash)
}

// BondRecovery is a free data retrieval call binding the contract method 0x2cf66e56.
//
// Solidity: function bondRecovery() view returns(uint8 disposition, address claimer, uint256 payment)
func (_ITournament *ITournamentCaller) BondRecovery(opts *bind.CallOpts) (struct {
	Disposition uint8
	Claimer     common.Address
	Payment     *big.Int
}, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "bondRecovery")

	outstruct := new(struct {
		Disposition uint8
		Claimer     common.Address
		Payment     *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Disposition = *abi.ConvertType(out[0], new(uint8)).(*uint8)
	outstruct.Claimer = *abi.ConvertType(out[1], new(common.Address)).(*common.Address)
	outstruct.Payment = *abi.ConvertType(out[2], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// BondRecovery is a free data retrieval call binding the contract method 0x2cf66e56.
//
// Solidity: function bondRecovery() view returns(uint8 disposition, address claimer, uint256 payment)
func (_ITournament *ITournamentSession) BondRecovery() (struct {
	Disposition uint8
	Claimer     common.Address
	Payment     *big.Int
}, error) {
	return _ITournament.Contract.BondRecovery(&_ITournament.CallOpts)
}

// BondRecovery is a free data retrieval call binding the contract method 0x2cf66e56.
//
// Solidity: function bondRecovery() view returns(uint8 disposition, address claimer, uint256 payment)
func (_ITournament *ITournamentCallerSession) BondRecovery() (struct {
	Disposition uint8
	Claimer     common.Address
	Payment     *big.Int
}, error) {
	return _ITournament.Contract.BondRecovery(&_ITournament.CallOpts)
}

// BondValue is a free data retrieval call binding the contract method 0xd2d5862c.
//
// Solidity: function bondValue() view returns(uint256)
func (_ITournament *ITournamentCaller) BondValue(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "bondValue")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// BondValue is a free data retrieval call binding the contract method 0xd2d5862c.
//
// Solidity: function bondValue() view returns(uint256)
func (_ITournament *ITournamentSession) BondValue() (*big.Int, error) {
	return _ITournament.Contract.BondValue(&_ITournament.CallOpts)
}

// BondValue is a free data retrieval call binding the contract method 0xd2d5862c.
//
// Solidity: function bondValue() view returns(uint256)
func (_ITournament *ITournamentCallerSession) BondValue() (*big.Int, error) {
	return _ITournament.Contract.BondValue(&_ITournament.CallOpts)
}

// ClassifyMatchTimeout is a free data retrieval call binding the contract method 0xdcead735.
//
// Solidity: function classifyMatchTimeout((bytes32,bytes32) matchId) view returns(uint8 actualPhase, uint8 outcome, uint64 deferredCharge)
func (_ITournament *ITournamentCaller) ClassifyMatchTimeout(opts *bind.CallOpts, matchId MatchId) (struct {
	ActualPhase    uint8
	Outcome        uint8
	DeferredCharge uint64
}, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "classifyMatchTimeout", matchId)

	outstruct := new(struct {
		ActualPhase    uint8
		Outcome        uint8
		DeferredCharge uint64
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.ActualPhase = *abi.ConvertType(out[0], new(uint8)).(*uint8)
	outstruct.Outcome = *abi.ConvertType(out[1], new(uint8)).(*uint8)
	outstruct.DeferredCharge = *abi.ConvertType(out[2], new(uint64)).(*uint64)

	return *outstruct, err

}

// ClassifyMatchTimeout is a free data retrieval call binding the contract method 0xdcead735.
//
// Solidity: function classifyMatchTimeout((bytes32,bytes32) matchId) view returns(uint8 actualPhase, uint8 outcome, uint64 deferredCharge)
func (_ITournament *ITournamentSession) ClassifyMatchTimeout(matchId MatchId) (struct {
	ActualPhase    uint8
	Outcome        uint8
	DeferredCharge uint64
}, error) {
	return _ITournament.Contract.ClassifyMatchTimeout(&_ITournament.CallOpts, matchId)
}

// ClassifyMatchTimeout is a free data retrieval call binding the contract method 0xdcead735.
//
// Solidity: function classifyMatchTimeout((bytes32,bytes32) matchId) view returns(uint8 actualPhase, uint8 outcome, uint64 deferredCharge)
func (_ITournament *ITournamentCallerSession) ClassifyMatchTimeout(matchId MatchId) (struct {
	ActualPhase    uint8
	Outcome        uint8
	DeferredCharge uint64
}, error) {
	return _ITournament.Contract.ClassifyMatchTimeout(&_ITournament.CallOpts, matchId)
}

// CommitmentStanding is a free data retrieval call binding the contract method 0xf3cb94fc.
//
// Solidity: function commitmentStanding(bytes32 commitmentRoot) view returns((bool,bytes32,address,bool,uint64,uint64))
func (_ITournament *ITournamentCaller) CommitmentStanding(opts *bind.CallOpts, commitmentRoot [32]byte) (ITournamentCommitmentStandingView, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "commitmentStanding", commitmentRoot)

	if err != nil {
		return *new(ITournamentCommitmentStandingView), err
	}

	out0 := *abi.ConvertType(out[0], new(ITournamentCommitmentStandingView)).(*ITournamentCommitmentStandingView)

	return out0, err

}

// CommitmentStanding is a free data retrieval call binding the contract method 0xf3cb94fc.
//
// Solidity: function commitmentStanding(bytes32 commitmentRoot) view returns((bool,bytes32,address,bool,uint64,uint64))
func (_ITournament *ITournamentSession) CommitmentStanding(commitmentRoot [32]byte) (ITournamentCommitmentStandingView, error) {
	return _ITournament.Contract.CommitmentStanding(&_ITournament.CallOpts, commitmentRoot)
}

// CommitmentStanding is a free data retrieval call binding the contract method 0xf3cb94fc.
//
// Solidity: function commitmentStanding(bytes32 commitmentRoot) view returns((bool,bytes32,address,bool,uint64,uint64))
func (_ITournament *ITournamentCallerSession) CommitmentStanding(commitmentRoot [32]byte) (ITournamentCommitmentStandingView, error) {
	return _ITournament.Contract.CommitmentStanding(&_ITournament.CallOpts, commitmentRoot)
}

// GetCommitmentJoinedCount is a free data retrieval call binding the contract method 0x2c243a1e.
//
// Solidity: function getCommitmentJoinedCount() view returns(uint256)
func (_ITournament *ITournamentCaller) GetCommitmentJoinedCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "getCommitmentJoinedCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetCommitmentJoinedCount is a free data retrieval call binding the contract method 0x2c243a1e.
//
// Solidity: function getCommitmentJoinedCount() view returns(uint256)
func (_ITournament *ITournamentSession) GetCommitmentJoinedCount() (*big.Int, error) {
	return _ITournament.Contract.GetCommitmentJoinedCount(&_ITournament.CallOpts)
}

// GetCommitmentJoinedCount is a free data retrieval call binding the contract method 0x2c243a1e.
//
// Solidity: function getCommitmentJoinedCount() view returns(uint256)
func (_ITournament *ITournamentCallerSession) GetCommitmentJoinedCount() (*big.Int, error) {
	return _ITournament.Contract.GetCommitmentJoinedCount(&_ITournament.CallOpts)
}

// GetLeafMatchSealedCount is a free data retrieval call binding the contract method 0x87373745.
//
// Solidity: function getLeafMatchSealedCount() view returns(uint256)
func (_ITournament *ITournamentCaller) GetLeafMatchSealedCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "getLeafMatchSealedCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetLeafMatchSealedCount is a free data retrieval call binding the contract method 0x87373745.
//
// Solidity: function getLeafMatchSealedCount() view returns(uint256)
func (_ITournament *ITournamentSession) GetLeafMatchSealedCount() (*big.Int, error) {
	return _ITournament.Contract.GetLeafMatchSealedCount(&_ITournament.CallOpts)
}

// GetLeafMatchSealedCount is a free data retrieval call binding the contract method 0x87373745.
//
// Solidity: function getLeafMatchSealedCount() view returns(uint256)
func (_ITournament *ITournamentCallerSession) GetLeafMatchSealedCount() (*big.Int, error) {
	return _ITournament.Contract.GetLeafMatchSealedCount(&_ITournament.CallOpts)
}

// GetMatchAdvancedCount is a free data retrieval call binding the contract method 0xf8cb3bd0.
//
// Solidity: function getMatchAdvancedCount() view returns(uint256)
func (_ITournament *ITournamentCaller) GetMatchAdvancedCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "getMatchAdvancedCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetMatchAdvancedCount is a free data retrieval call binding the contract method 0xf8cb3bd0.
//
// Solidity: function getMatchAdvancedCount() view returns(uint256)
func (_ITournament *ITournamentSession) GetMatchAdvancedCount() (*big.Int, error) {
	return _ITournament.Contract.GetMatchAdvancedCount(&_ITournament.CallOpts)
}

// GetMatchAdvancedCount is a free data retrieval call binding the contract method 0xf8cb3bd0.
//
// Solidity: function getMatchAdvancedCount() view returns(uint256)
func (_ITournament *ITournamentCallerSession) GetMatchAdvancedCount() (*big.Int, error) {
	return _ITournament.Contract.GetMatchAdvancedCount(&_ITournament.CallOpts)
}

// GetMatchCreatedCount is a free data retrieval call binding the contract method 0x8989ced4.
//
// Solidity: function getMatchCreatedCount() view returns(uint256)
func (_ITournament *ITournamentCaller) GetMatchCreatedCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "getMatchCreatedCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetMatchCreatedCount is a free data retrieval call binding the contract method 0x8989ced4.
//
// Solidity: function getMatchCreatedCount() view returns(uint256)
func (_ITournament *ITournamentSession) GetMatchCreatedCount() (*big.Int, error) {
	return _ITournament.Contract.GetMatchCreatedCount(&_ITournament.CallOpts)
}

// GetMatchCreatedCount is a free data retrieval call binding the contract method 0x8989ced4.
//
// Solidity: function getMatchCreatedCount() view returns(uint256)
func (_ITournament *ITournamentCallerSession) GetMatchCreatedCount() (*big.Int, error) {
	return _ITournament.Contract.GetMatchCreatedCount(&_ITournament.CallOpts)
}

// GetMatchDeletedCount is a free data retrieval call binding the contract method 0xd3976945.
//
// Solidity: function getMatchDeletedCount() view returns(uint256)
func (_ITournament *ITournamentCaller) GetMatchDeletedCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "getMatchDeletedCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetMatchDeletedCount is a free data retrieval call binding the contract method 0xd3976945.
//
// Solidity: function getMatchDeletedCount() view returns(uint256)
func (_ITournament *ITournamentSession) GetMatchDeletedCount() (*big.Int, error) {
	return _ITournament.Contract.GetMatchDeletedCount(&_ITournament.CallOpts)
}

// GetMatchDeletedCount is a free data retrieval call binding the contract method 0xd3976945.
//
// Solidity: function getMatchDeletedCount() view returns(uint256)
func (_ITournament *ITournamentCallerSession) GetMatchDeletedCount() (*big.Int, error) {
	return _ITournament.Contract.GetMatchDeletedCount(&_ITournament.CallOpts)
}

// GetNewInnerTournamentCount is a free data retrieval call binding the contract method 0x30beca25.
//
// Solidity: function getNewInnerTournamentCount() view returns(uint256)
func (_ITournament *ITournamentCaller) GetNewInnerTournamentCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "getNewInnerTournamentCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNewInnerTournamentCount is a free data retrieval call binding the contract method 0x30beca25.
//
// Solidity: function getNewInnerTournamentCount() view returns(uint256)
func (_ITournament *ITournamentSession) GetNewInnerTournamentCount() (*big.Int, error) {
	return _ITournament.Contract.GetNewInnerTournamentCount(&_ITournament.CallOpts)
}

// GetNewInnerTournamentCount is a free data retrieval call binding the contract method 0x30beca25.
//
// Solidity: function getNewInnerTournamentCount() view returns(uint256)
func (_ITournament *ITournamentCallerSession) GetNewInnerTournamentCount() (*big.Int, error) {
	return _ITournament.Contract.GetNewInnerTournamentCount(&_ITournament.CallOpts)
}

// InnerResult is a free data retrieval call binding the contract method 0x08f0f877.
//
// Solidity: function innerResult() view returns((uint8,bytes32,uint64))
func (_ITournament *ITournamentCaller) InnerResult(opts *bind.CallOpts) (ITournamentInnerResultView, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "innerResult")

	if err != nil {
		return *new(ITournamentInnerResultView), err
	}

	out0 := *abi.ConvertType(out[0], new(ITournamentInnerResultView)).(*ITournamentInnerResultView)

	return out0, err

}

// InnerResult is a free data retrieval call binding the contract method 0x08f0f877.
//
// Solidity: function innerResult() view returns((uint8,bytes32,uint64))
func (_ITournament *ITournamentSession) InnerResult() (ITournamentInnerResultView, error) {
	return _ITournament.Contract.InnerResult(&_ITournament.CallOpts)
}

// InnerResult is a free data retrieval call binding the contract method 0x08f0f877.
//
// Solidity: function innerResult() view returns((uint8,bytes32,uint64))
func (_ITournament *ITournamentCallerSession) InnerResult() (ITournamentInnerResultView, error) {
	return _ITournament.Contract.InnerResult(&_ITournament.CallOpts)
}

// ReadyToSealMatch is a free data retrieval call binding the contract method 0x62d0d040.
//
// Solidity: function readyToSealMatch(bytes32 matchIdHash) view returns(uint8 actualPhase, (bytes32,bytes32,bytes32,uint256,uint256,uint8) value)
func (_ITournament *ITournamentCaller) ReadyToSealMatch(opts *bind.CallOpts, matchIdHash [32]byte) (struct {
	ActualPhase uint8
	Value       ITournamentReadyToSealMatchView
}, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "readyToSealMatch", matchIdHash)

	outstruct := new(struct {
		ActualPhase uint8
		Value       ITournamentReadyToSealMatchView
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.ActualPhase = *abi.ConvertType(out[0], new(uint8)).(*uint8)
	outstruct.Value = *abi.ConvertType(out[1], new(ITournamentReadyToSealMatchView)).(*ITournamentReadyToSealMatchView)

	return *outstruct, err

}

// ReadyToSealMatch is a free data retrieval call binding the contract method 0x62d0d040.
//
// Solidity: function readyToSealMatch(bytes32 matchIdHash) view returns(uint8 actualPhase, (bytes32,bytes32,bytes32,uint256,uint256,uint8) value)
func (_ITournament *ITournamentSession) ReadyToSealMatch(matchIdHash [32]byte) (struct {
	ActualPhase uint8
	Value       ITournamentReadyToSealMatchView
}, error) {
	return _ITournament.Contract.ReadyToSealMatch(&_ITournament.CallOpts, matchIdHash)
}

// ReadyToSealMatch is a free data retrieval call binding the contract method 0x62d0d040.
//
// Solidity: function readyToSealMatch(bytes32 matchIdHash) view returns(uint8 actualPhase, (bytes32,bytes32,bytes32,uint256,uint256,uint8) value)
func (_ITournament *ITournamentCallerSession) ReadyToSealMatch(matchIdHash [32]byte) (struct {
	ActualPhase uint8
	Value       ITournamentReadyToSealMatchView
}, error) {
	return _ITournament.Contract.ReadyToSealMatch(&_ITournament.CallOpts, matchIdHash)
}

// SealedMatch is a free data retrieval call binding the contract method 0x6e42fc59.
//
// Solidity: function sealedMatch(bytes32 matchIdHash) view returns(uint8 actualPhase, (bytes32,uint256,uint256,bytes32,bytes32) value)
func (_ITournament *ITournamentCaller) SealedMatch(opts *bind.CallOpts, matchIdHash [32]byte) (struct {
	ActualPhase uint8
	Value       ITournamentSealedMatchView
}, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "sealedMatch", matchIdHash)

	outstruct := new(struct {
		ActualPhase uint8
		Value       ITournamentSealedMatchView
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.ActualPhase = *abi.ConvertType(out[0], new(uint8)).(*uint8)
	outstruct.Value = *abi.ConvertType(out[1], new(ITournamentSealedMatchView)).(*ITournamentSealedMatchView)

	return *outstruct, err

}

// SealedMatch is a free data retrieval call binding the contract method 0x6e42fc59.
//
// Solidity: function sealedMatch(bytes32 matchIdHash) view returns(uint8 actualPhase, (bytes32,uint256,uint256,bytes32,bytes32) value)
func (_ITournament *ITournamentSession) SealedMatch(matchIdHash [32]byte) (struct {
	ActualPhase uint8
	Value       ITournamentSealedMatchView
}, error) {
	return _ITournament.Contract.SealedMatch(&_ITournament.CallOpts, matchIdHash)
}

// SealedMatch is a free data retrieval call binding the contract method 0x6e42fc59.
//
// Solidity: function sealedMatch(bytes32 matchIdHash) view returns(uint8 actualPhase, (bytes32,uint256,uint256,bytes32,bytes32) value)
func (_ITournament *ITournamentCallerSession) SealedMatch(matchIdHash [32]byte) (struct {
	ActualPhase uint8
	Value       ITournamentSealedMatchView
}, error) {
	return _ITournament.Contract.SealedMatch(&_ITournament.CallOpts, matchIdHash)
}

// TournamentDescriptor is a free data retrieval call binding the contract method 0x21719b1e.
//
// Solidity: function tournamentDescriptor() view returns((bytes32,uint256,uint64,uint64,uint64,uint8,uint64,uint64))
func (_ITournament *ITournamentCaller) TournamentDescriptor(opts *bind.CallOpts) (ITournamentTournamentDescriptor, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "tournamentDescriptor")

	if err != nil {
		return *new(ITournamentTournamentDescriptor), err
	}

	out0 := *abi.ConvertType(out[0], new(ITournamentTournamentDescriptor)).(*ITournamentTournamentDescriptor)

	return out0, err

}

// TournamentDescriptor is a free data retrieval call binding the contract method 0x21719b1e.
//
// Solidity: function tournamentDescriptor() view returns((bytes32,uint256,uint64,uint64,uint64,uint8,uint64,uint64))
func (_ITournament *ITournamentSession) TournamentDescriptor() (ITournamentTournamentDescriptor, error) {
	return _ITournament.Contract.TournamentDescriptor(&_ITournament.CallOpts)
}

// TournamentDescriptor is a free data retrieval call binding the contract method 0x21719b1e.
//
// Solidity: function tournamentDescriptor() view returns((bytes32,uint256,uint64,uint64,uint64,uint8,uint64,uint64))
func (_ITournament *ITournamentCallerSession) TournamentDescriptor() (ITournamentTournamentDescriptor, error) {
	return _ITournament.Contract.TournamentDescriptor(&_ITournament.CallOpts)
}

// TournamentStanding is a free data retrieval call binding the contract method 0x0589deec.
//
// Solidity: function tournamentStanding() view returns((uint8,bool,bool,bytes32,bytes32,bytes32,uint64,uint64))
func (_ITournament *ITournamentCaller) TournamentStanding(opts *bind.CallOpts) (ITournamentTournamentStandingView, error) {
	var out []interface{}
	err := _ITournament.contract.Call(opts, &out, "tournamentStanding")

	if err != nil {
		return *new(ITournamentTournamentStandingView), err
	}

	out0 := *abi.ConvertType(out[0], new(ITournamentTournamentStandingView)).(*ITournamentTournamentStandingView)

	return out0, err

}

// TournamentStanding is a free data retrieval call binding the contract method 0x0589deec.
//
// Solidity: function tournamentStanding() view returns((uint8,bool,bool,bytes32,bytes32,bytes32,uint64,uint64))
func (_ITournament *ITournamentSession) TournamentStanding() (ITournamentTournamentStandingView, error) {
	return _ITournament.Contract.TournamentStanding(&_ITournament.CallOpts)
}

// TournamentStanding is a free data retrieval call binding the contract method 0x0589deec.
//
// Solidity: function tournamentStanding() view returns((uint8,bool,bool,bytes32,bytes32,bytes32,uint64,uint64))
func (_ITournament *ITournamentCallerSession) TournamentStanding() (ITournamentTournamentStandingView, error) {
	return _ITournament.Contract.TournamentStanding(&_ITournament.CallOpts)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) matchId, bytes32 leftNode, bytes32 rightNode, bytes32 newLeftNode, bytes32 newRightNode) returns()
func (_ITournament *ITournamentTransactor) AdvanceMatch(opts *bind.TransactOpts, matchId MatchId, leftNode [32]byte, rightNode [32]byte, newLeftNode [32]byte, newRightNode [32]byte) (*types.Transaction, error) {
	return _ITournament.contract.Transact(opts, "advanceMatch", matchId, leftNode, rightNode, newLeftNode, newRightNode)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) matchId, bytes32 leftNode, bytes32 rightNode, bytes32 newLeftNode, bytes32 newRightNode) returns()
func (_ITournament *ITournamentSession) AdvanceMatch(matchId MatchId, leftNode [32]byte, rightNode [32]byte, newLeftNode [32]byte, newRightNode [32]byte) (*types.Transaction, error) {
	return _ITournament.Contract.AdvanceMatch(&_ITournament.TransactOpts, matchId, leftNode, rightNode, newLeftNode, newRightNode)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) matchId, bytes32 leftNode, bytes32 rightNode, bytes32 newLeftNode, bytes32 newRightNode) returns()
func (_ITournament *ITournamentTransactorSession) AdvanceMatch(matchId MatchId, leftNode [32]byte, rightNode [32]byte, newLeftNode [32]byte, newRightNode [32]byte) (*types.Transaction, error) {
	return _ITournament.Contract.AdvanceMatch(&_ITournament.TransactOpts, matchId, leftNode, rightNode, newLeftNode, newRightNode)
}

// EliminateInnerTournament is a paid mutator transaction binding the contract method 0x26860b49.
//
// Solidity: function eliminateInnerTournament(address childTournament) returns()
func (_ITournament *ITournamentTransactor) EliminateInnerTournament(opts *bind.TransactOpts, childTournament common.Address) (*types.Transaction, error) {
	return _ITournament.contract.Transact(opts, "eliminateInnerTournament", childTournament)
}

// EliminateInnerTournament is a paid mutator transaction binding the contract method 0x26860b49.
//
// Solidity: function eliminateInnerTournament(address childTournament) returns()
func (_ITournament *ITournamentSession) EliminateInnerTournament(childTournament common.Address) (*types.Transaction, error) {
	return _ITournament.Contract.EliminateInnerTournament(&_ITournament.TransactOpts, childTournament)
}

// EliminateInnerTournament is a paid mutator transaction binding the contract method 0x26860b49.
//
// Solidity: function eliminateInnerTournament(address childTournament) returns()
func (_ITournament *ITournamentTransactorSession) EliminateInnerTournament(childTournament common.Address) (*types.Transaction, error) {
	return _ITournament.Contract.EliminateInnerTournament(&_ITournament.TransactOpts, childTournament)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) matchId) returns()
func (_ITournament *ITournamentTransactor) EliminateMatchByTimeout(opts *bind.TransactOpts, matchId MatchId) (*types.Transaction, error) {
	return _ITournament.contract.Transact(opts, "eliminateMatchByTimeout", matchId)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) matchId) returns()
func (_ITournament *ITournamentSession) EliminateMatchByTimeout(matchId MatchId) (*types.Transaction, error) {
	return _ITournament.Contract.EliminateMatchByTimeout(&_ITournament.TransactOpts, matchId)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) matchId) returns()
func (_ITournament *ITournamentTransactorSession) EliminateMatchByTimeout(matchId MatchId) (*types.Transaction, error) {
	return _ITournament.Contract.EliminateMatchByTimeout(&_ITournament.TransactOpts, matchId)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 finalState, bytes32[] proof, bytes32 leftNode, bytes32 rightNode) payable returns()
func (_ITournament *ITournamentTransactor) JoinTournament(opts *bind.TransactOpts, finalState [32]byte, proof [][32]byte, leftNode [32]byte, rightNode [32]byte) (*types.Transaction, error) {
	return _ITournament.contract.Transact(opts, "joinTournament", finalState, proof, leftNode, rightNode)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 finalState, bytes32[] proof, bytes32 leftNode, bytes32 rightNode) payable returns()
func (_ITournament *ITournamentSession) JoinTournament(finalState [32]byte, proof [][32]byte, leftNode [32]byte, rightNode [32]byte) (*types.Transaction, error) {
	return _ITournament.Contract.JoinTournament(&_ITournament.TransactOpts, finalState, proof, leftNode, rightNode)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 finalState, bytes32[] proof, bytes32 leftNode, bytes32 rightNode) payable returns()
func (_ITournament *ITournamentTransactorSession) JoinTournament(finalState [32]byte, proof [][32]byte, leftNode [32]byte, rightNode [32]byte) (*types.Transaction, error) {
	return _ITournament.Contract.JoinTournament(&_ITournament.TransactOpts, finalState, proof, leftNode, rightNode)
}

// SealInnerMatchAndCreateInnerTournament is a paid mutator transaction binding the contract method 0x3f36e2fe.
//
// Solidity: function sealInnerMatchAndCreateInnerTournament((bytes32,bytes32) matchId, bytes32 leftLeaf, bytes32 rightLeaf, bytes32 agreeHash, bytes32[] agreeHashProof) returns()
func (_ITournament *ITournamentTransactor) SealInnerMatchAndCreateInnerTournament(opts *bind.TransactOpts, matchId MatchId, leftLeaf [32]byte, rightLeaf [32]byte, agreeHash [32]byte, agreeHashProof [][32]byte) (*types.Transaction, error) {
	return _ITournament.contract.Transact(opts, "sealInnerMatchAndCreateInnerTournament", matchId, leftLeaf, rightLeaf, agreeHash, agreeHashProof)
}

// SealInnerMatchAndCreateInnerTournament is a paid mutator transaction binding the contract method 0x3f36e2fe.
//
// Solidity: function sealInnerMatchAndCreateInnerTournament((bytes32,bytes32) matchId, bytes32 leftLeaf, bytes32 rightLeaf, bytes32 agreeHash, bytes32[] agreeHashProof) returns()
func (_ITournament *ITournamentSession) SealInnerMatchAndCreateInnerTournament(matchId MatchId, leftLeaf [32]byte, rightLeaf [32]byte, agreeHash [32]byte, agreeHashProof [][32]byte) (*types.Transaction, error) {
	return _ITournament.Contract.SealInnerMatchAndCreateInnerTournament(&_ITournament.TransactOpts, matchId, leftLeaf, rightLeaf, agreeHash, agreeHashProof)
}

// SealInnerMatchAndCreateInnerTournament is a paid mutator transaction binding the contract method 0x3f36e2fe.
//
// Solidity: function sealInnerMatchAndCreateInnerTournament((bytes32,bytes32) matchId, bytes32 leftLeaf, bytes32 rightLeaf, bytes32 agreeHash, bytes32[] agreeHashProof) returns()
func (_ITournament *ITournamentTransactorSession) SealInnerMatchAndCreateInnerTournament(matchId MatchId, leftLeaf [32]byte, rightLeaf [32]byte, agreeHash [32]byte, agreeHashProof [][32]byte) (*types.Transaction, error) {
	return _ITournament.Contract.SealInnerMatchAndCreateInnerTournament(&_ITournament.TransactOpts, matchId, leftLeaf, rightLeaf, agreeHash, agreeHashProof)
}

// SealLeafMatch is a paid mutator transaction binding the contract method 0x5017746a.
//
// Solidity: function sealLeafMatch((bytes32,bytes32) matchId, bytes32 leftLeaf, bytes32 rightLeaf, bytes32 agreeHash, bytes32[] agreeHashProof) returns()
func (_ITournament *ITournamentTransactor) SealLeafMatch(opts *bind.TransactOpts, matchId MatchId, leftLeaf [32]byte, rightLeaf [32]byte, agreeHash [32]byte, agreeHashProof [][32]byte) (*types.Transaction, error) {
	return _ITournament.contract.Transact(opts, "sealLeafMatch", matchId, leftLeaf, rightLeaf, agreeHash, agreeHashProof)
}

// SealLeafMatch is a paid mutator transaction binding the contract method 0x5017746a.
//
// Solidity: function sealLeafMatch((bytes32,bytes32) matchId, bytes32 leftLeaf, bytes32 rightLeaf, bytes32 agreeHash, bytes32[] agreeHashProof) returns()
func (_ITournament *ITournamentSession) SealLeafMatch(matchId MatchId, leftLeaf [32]byte, rightLeaf [32]byte, agreeHash [32]byte, agreeHashProof [][32]byte) (*types.Transaction, error) {
	return _ITournament.Contract.SealLeafMatch(&_ITournament.TransactOpts, matchId, leftLeaf, rightLeaf, agreeHash, agreeHashProof)
}

// SealLeafMatch is a paid mutator transaction binding the contract method 0x5017746a.
//
// Solidity: function sealLeafMatch((bytes32,bytes32) matchId, bytes32 leftLeaf, bytes32 rightLeaf, bytes32 agreeHash, bytes32[] agreeHashProof) returns()
func (_ITournament *ITournamentTransactorSession) SealLeafMatch(matchId MatchId, leftLeaf [32]byte, rightLeaf [32]byte, agreeHash [32]byte, agreeHashProof [][32]byte) (*types.Transaction, error) {
	return _ITournament.Contract.SealLeafMatch(&_ITournament.TransactOpts, matchId, leftLeaf, rightLeaf, agreeHash, agreeHashProof)
}

// TryRecoveringBond is a paid mutator transaction binding the contract method 0x33807cbc.
//
// Solidity: function tryRecoveringBond() returns(bool)
func (_ITournament *ITournamentTransactor) TryRecoveringBond(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ITournament.contract.Transact(opts, "tryRecoveringBond")
}

// TryRecoveringBond is a paid mutator transaction binding the contract method 0x33807cbc.
//
// Solidity: function tryRecoveringBond() returns(bool)
func (_ITournament *ITournamentSession) TryRecoveringBond() (*types.Transaction, error) {
	return _ITournament.Contract.TryRecoveringBond(&_ITournament.TransactOpts)
}

// TryRecoveringBond is a paid mutator transaction binding the contract method 0x33807cbc.
//
// Solidity: function tryRecoveringBond() returns(bool)
func (_ITournament *ITournamentTransactorSession) TryRecoveringBond() (*types.Transaction, error) {
	return _ITournament.Contract.TryRecoveringBond(&_ITournament.TransactOpts)
}

// WinInnerTournament is a paid mutator transaction binding the contract method 0x4a95153e.
//
// Solidity: function winInnerTournament(address childTournament, bytes32 leftNode, bytes32 rightNode) returns()
func (_ITournament *ITournamentTransactor) WinInnerTournament(opts *bind.TransactOpts, childTournament common.Address, leftNode [32]byte, rightNode [32]byte) (*types.Transaction, error) {
	return _ITournament.contract.Transact(opts, "winInnerTournament", childTournament, leftNode, rightNode)
}

// WinInnerTournament is a paid mutator transaction binding the contract method 0x4a95153e.
//
// Solidity: function winInnerTournament(address childTournament, bytes32 leftNode, bytes32 rightNode) returns()
func (_ITournament *ITournamentSession) WinInnerTournament(childTournament common.Address, leftNode [32]byte, rightNode [32]byte) (*types.Transaction, error) {
	return _ITournament.Contract.WinInnerTournament(&_ITournament.TransactOpts, childTournament, leftNode, rightNode)
}

// WinInnerTournament is a paid mutator transaction binding the contract method 0x4a95153e.
//
// Solidity: function winInnerTournament(address childTournament, bytes32 leftNode, bytes32 rightNode) returns()
func (_ITournament *ITournamentTransactorSession) WinInnerTournament(childTournament common.Address, leftNode [32]byte, rightNode [32]byte) (*types.Transaction, error) {
	return _ITournament.Contract.WinInnerTournament(&_ITournament.TransactOpts, childTournament, leftNode, rightNode)
}

// WinLeafMatch is a paid mutator transaction binding the contract method 0x6041ddd5.
//
// Solidity: function winLeafMatch((bytes32,bytes32) matchId, bytes32 leftNode, bytes32 rightNode, bytes proofs) returns()
func (_ITournament *ITournamentTransactor) WinLeafMatch(opts *bind.TransactOpts, matchId MatchId, leftNode [32]byte, rightNode [32]byte, proofs []byte) (*types.Transaction, error) {
	return _ITournament.contract.Transact(opts, "winLeafMatch", matchId, leftNode, rightNode, proofs)
}

// WinLeafMatch is a paid mutator transaction binding the contract method 0x6041ddd5.
//
// Solidity: function winLeafMatch((bytes32,bytes32) matchId, bytes32 leftNode, bytes32 rightNode, bytes proofs) returns()
func (_ITournament *ITournamentSession) WinLeafMatch(matchId MatchId, leftNode [32]byte, rightNode [32]byte, proofs []byte) (*types.Transaction, error) {
	return _ITournament.Contract.WinLeafMatch(&_ITournament.TransactOpts, matchId, leftNode, rightNode, proofs)
}

// WinLeafMatch is a paid mutator transaction binding the contract method 0x6041ddd5.
//
// Solidity: function winLeafMatch((bytes32,bytes32) matchId, bytes32 leftNode, bytes32 rightNode, bytes proofs) returns()
func (_ITournament *ITournamentTransactorSession) WinLeafMatch(matchId MatchId, leftNode [32]byte, rightNode [32]byte, proofs []byte) (*types.Transaction, error) {
	return _ITournament.Contract.WinLeafMatch(&_ITournament.TransactOpts, matchId, leftNode, rightNode, proofs)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) matchId, bytes32 leftNode, bytes32 rightNode) returns()
func (_ITournament *ITournamentTransactor) WinMatchByTimeout(opts *bind.TransactOpts, matchId MatchId, leftNode [32]byte, rightNode [32]byte) (*types.Transaction, error) {
	return _ITournament.contract.Transact(opts, "winMatchByTimeout", matchId, leftNode, rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) matchId, bytes32 leftNode, bytes32 rightNode) returns()
func (_ITournament *ITournamentSession) WinMatchByTimeout(matchId MatchId, leftNode [32]byte, rightNode [32]byte) (*types.Transaction, error) {
	return _ITournament.Contract.WinMatchByTimeout(&_ITournament.TransactOpts, matchId, leftNode, rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) matchId, bytes32 leftNode, bytes32 rightNode) returns()
func (_ITournament *ITournamentTransactorSession) WinMatchByTimeout(matchId MatchId, leftNode [32]byte, rightNode [32]byte) (*types.Transaction, error) {
	return _ITournament.Contract.WinMatchByTimeout(&_ITournament.TransactOpts, matchId, leftNode, rightNode)
}

// ITournamentBondRecoveredIterator is returned from FilterBondRecovered and is used to iterate over the raw logs and unpacked data for BondRecovered events raised by the ITournament contract.
type ITournamentBondRecoveredIterator struct {
	Event *ITournamentBondRecovered // Event containing the contract specifics and raw log

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
func (it *ITournamentBondRecoveredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ITournamentBondRecovered)
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
		it.Event = new(ITournamentBondRecovered)
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
func (it *ITournamentBondRecoveredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ITournamentBondRecoveredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ITournamentBondRecovered represents a BondRecovered event raised by the ITournament contract.
type ITournamentBondRecovered struct {
	Commitment [32]byte
	Claimer    common.Address
	Payment    *big.Int
	Burned     *big.Int
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterBondRecovered is a free log retrieval operation binding the contract event 0x11bda32901e3e37ab2859f6a8b7b525ecd2b67c579f173700ad8642346cabf4e.
//
// Solidity: event BondRecovered(bytes32 indexed commitment, address indexed claimer, uint256 payment, uint256 burned)
func (_ITournament *ITournamentFilterer) FilterBondRecovered(opts *bind.FilterOpts, commitment [][32]byte, claimer []common.Address) (*ITournamentBondRecoveredIterator, error) {

	var commitmentRule []interface{}
	for _, commitmentItem := range commitment {
		commitmentRule = append(commitmentRule, commitmentItem)
	}
	var claimerRule []interface{}
	for _, claimerItem := range claimer {
		claimerRule = append(claimerRule, claimerItem)
	}

	logs, sub, err := _ITournament.contract.FilterLogs(opts, "BondRecovered", commitmentRule, claimerRule)
	if err != nil {
		return nil, err
	}
	return &ITournamentBondRecoveredIterator{contract: _ITournament.contract, event: "BondRecovered", logs: logs, sub: sub}, nil
}

// WatchBondRecovered is a free log subscription operation binding the contract event 0x11bda32901e3e37ab2859f6a8b7b525ecd2b67c579f173700ad8642346cabf4e.
//
// Solidity: event BondRecovered(bytes32 indexed commitment, address indexed claimer, uint256 payment, uint256 burned)
func (_ITournament *ITournamentFilterer) WatchBondRecovered(opts *bind.WatchOpts, sink chan<- *ITournamentBondRecovered, commitment [][32]byte, claimer []common.Address) (event.Subscription, error) {

	var commitmentRule []interface{}
	for _, commitmentItem := range commitment {
		commitmentRule = append(commitmentRule, commitmentItem)
	}
	var claimerRule []interface{}
	for _, claimerItem := range claimer {
		claimerRule = append(claimerRule, claimerItem)
	}

	logs, sub, err := _ITournament.contract.WatchLogs(opts, "BondRecovered", commitmentRule, claimerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ITournamentBondRecovered)
				if err := _ITournament.contract.UnpackLog(event, "BondRecovered", log); err != nil {
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

// ParseBondRecovered is a log parse operation binding the contract event 0x11bda32901e3e37ab2859f6a8b7b525ecd2b67c579f173700ad8642346cabf4e.
//
// Solidity: event BondRecovered(bytes32 indexed commitment, address indexed claimer, uint256 payment, uint256 burned)
func (_ITournament *ITournamentFilterer) ParseBondRecovered(log types.Log) (*ITournamentBondRecovered, error) {
	event := new(ITournamentBondRecovered)
	if err := _ITournament.contract.UnpackLog(event, "BondRecovered", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ITournamentCommitmentJoinedIterator is returned from FilterCommitmentJoined and is used to iterate over the raw logs and unpacked data for CommitmentJoined events raised by the ITournament contract.
type ITournamentCommitmentJoinedIterator struct {
	Event *ITournamentCommitmentJoined // Event containing the contract specifics and raw log

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
func (it *ITournamentCommitmentJoinedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ITournamentCommitmentJoined)
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
		it.Event = new(ITournamentCommitmentJoined)
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
func (it *ITournamentCommitmentJoinedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ITournamentCommitmentJoinedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ITournamentCommitmentJoined represents a CommitmentJoined event raised by the ITournament contract.
type ITournamentCommitmentJoined struct {
	Commitment     [32]byte
	FinalStateHash [32]byte
	Submitter      common.Address
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterCommitmentJoined is a free log retrieval operation binding the contract event 0xf8e98e9201e0caf973fa5520838a058bd8a819e0a8f5dd1fa08c3e550d4b9872.
//
// Solidity: event CommitmentJoined(bytes32 indexed commitment, bytes32 finalStateHash, address indexed submitter)
func (_ITournament *ITournamentFilterer) FilterCommitmentJoined(opts *bind.FilterOpts, commitment [][32]byte, submitter []common.Address) (*ITournamentCommitmentJoinedIterator, error) {

	var commitmentRule []interface{}
	for _, commitmentItem := range commitment {
		commitmentRule = append(commitmentRule, commitmentItem)
	}

	var submitterRule []interface{}
	for _, submitterItem := range submitter {
		submitterRule = append(submitterRule, submitterItem)
	}

	logs, sub, err := _ITournament.contract.FilterLogs(opts, "CommitmentJoined", commitmentRule, submitterRule)
	if err != nil {
		return nil, err
	}
	return &ITournamentCommitmentJoinedIterator{contract: _ITournament.contract, event: "CommitmentJoined", logs: logs, sub: sub}, nil
}

// WatchCommitmentJoined is a free log subscription operation binding the contract event 0xf8e98e9201e0caf973fa5520838a058bd8a819e0a8f5dd1fa08c3e550d4b9872.
//
// Solidity: event CommitmentJoined(bytes32 indexed commitment, bytes32 finalStateHash, address indexed submitter)
func (_ITournament *ITournamentFilterer) WatchCommitmentJoined(opts *bind.WatchOpts, sink chan<- *ITournamentCommitmentJoined, commitment [][32]byte, submitter []common.Address) (event.Subscription, error) {

	var commitmentRule []interface{}
	for _, commitmentItem := range commitment {
		commitmentRule = append(commitmentRule, commitmentItem)
	}

	var submitterRule []interface{}
	for _, submitterItem := range submitter {
		submitterRule = append(submitterRule, submitterItem)
	}

	logs, sub, err := _ITournament.contract.WatchLogs(opts, "CommitmentJoined", commitmentRule, submitterRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ITournamentCommitmentJoined)
				if err := _ITournament.contract.UnpackLog(event, "CommitmentJoined", log); err != nil {
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

// ParseCommitmentJoined is a log parse operation binding the contract event 0xf8e98e9201e0caf973fa5520838a058bd8a819e0a8f5dd1fa08c3e550d4b9872.
//
// Solidity: event CommitmentJoined(bytes32 indexed commitment, bytes32 finalStateHash, address indexed submitter)
func (_ITournament *ITournamentFilterer) ParseCommitmentJoined(log types.Log) (*ITournamentCommitmentJoined, error) {
	event := new(ITournamentCommitmentJoined)
	if err := _ITournament.contract.UnpackLog(event, "CommitmentJoined", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ITournamentLeafMatchSealedIterator is returned from FilterLeafMatchSealed and is used to iterate over the raw logs and unpacked data for LeafMatchSealed events raised by the ITournament contract.
type ITournamentLeafMatchSealedIterator struct {
	Event *ITournamentLeafMatchSealed // Event containing the contract specifics and raw log

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
func (it *ITournamentLeafMatchSealedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ITournamentLeafMatchSealed)
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
		it.Event = new(ITournamentLeafMatchSealed)
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
func (it *ITournamentLeafMatchSealedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ITournamentLeafMatchSealedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ITournamentLeafMatchSealed represents a LeafMatchSealed event raised by the ITournament contract.
type ITournamentLeafMatchSealed struct {
	MatchIdHash  [32]byte
	EliminableAt uint64
	Raw          types.Log // Blockchain specific contextual infos
}

// FilterLeafMatchSealed is a free log retrieval operation binding the contract event 0x304d9235f96c68167f2d1c8f6fbdce133d7d35b4706d00f47b40172cf96bd923.
//
// Solidity: event LeafMatchSealed(bytes32 indexed matchIdHash, uint64 eliminableAt)
func (_ITournament *ITournamentFilterer) FilterLeafMatchSealed(opts *bind.FilterOpts, matchIdHash [][32]byte) (*ITournamentLeafMatchSealedIterator, error) {

	var matchIdHashRule []interface{}
	for _, matchIdHashItem := range matchIdHash {
		matchIdHashRule = append(matchIdHashRule, matchIdHashItem)
	}

	logs, sub, err := _ITournament.contract.FilterLogs(opts, "LeafMatchSealed", matchIdHashRule)
	if err != nil {
		return nil, err
	}
	return &ITournamentLeafMatchSealedIterator{contract: _ITournament.contract, event: "LeafMatchSealed", logs: logs, sub: sub}, nil
}

// WatchLeafMatchSealed is a free log subscription operation binding the contract event 0x304d9235f96c68167f2d1c8f6fbdce133d7d35b4706d00f47b40172cf96bd923.
//
// Solidity: event LeafMatchSealed(bytes32 indexed matchIdHash, uint64 eliminableAt)
func (_ITournament *ITournamentFilterer) WatchLeafMatchSealed(opts *bind.WatchOpts, sink chan<- *ITournamentLeafMatchSealed, matchIdHash [][32]byte) (event.Subscription, error) {

	var matchIdHashRule []interface{}
	for _, matchIdHashItem := range matchIdHash {
		matchIdHashRule = append(matchIdHashRule, matchIdHashItem)
	}

	logs, sub, err := _ITournament.contract.WatchLogs(opts, "LeafMatchSealed", matchIdHashRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ITournamentLeafMatchSealed)
				if err := _ITournament.contract.UnpackLog(event, "LeafMatchSealed", log); err != nil {
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

// ParseLeafMatchSealed is a log parse operation binding the contract event 0x304d9235f96c68167f2d1c8f6fbdce133d7d35b4706d00f47b40172cf96bd923.
//
// Solidity: event LeafMatchSealed(bytes32 indexed matchIdHash, uint64 eliminableAt)
func (_ITournament *ITournamentFilterer) ParseLeafMatchSealed(log types.Log) (*ITournamentLeafMatchSealed, error) {
	event := new(ITournamentLeafMatchSealed)
	if err := _ITournament.contract.UnpackLog(event, "LeafMatchSealed", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ITournamentMatchAdvancedIterator is returned from FilterMatchAdvanced and is used to iterate over the raw logs and unpacked data for MatchAdvanced events raised by the ITournament contract.
type ITournamentMatchAdvancedIterator struct {
	Event *ITournamentMatchAdvanced // Event containing the contract specifics and raw log

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
func (it *ITournamentMatchAdvancedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ITournamentMatchAdvanced)
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
		it.Event = new(ITournamentMatchAdvanced)
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
func (it *ITournamentMatchAdvancedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ITournamentMatchAdvancedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ITournamentMatchAdvanced represents a MatchAdvanced event raised by the ITournament contract.
type ITournamentMatchAdvanced struct {
	MatchIdHash          [32]byte
	OtherParent          [32]byte
	LeftNode             [32]byte
	SegmentStartPosition *big.Int
	EliminableAt         uint64
	Raw                  types.Log // Blockchain specific contextual infos
}

// FilterMatchAdvanced is a free log retrieval operation binding the contract event 0xb6135d4f6f9687ce2f55ce0fd69c7f9d77e003a036ac8e402828abdd973621b1.
//
// Solidity: event MatchAdvanced(bytes32 indexed matchIdHash, bytes32 otherParent, bytes32 leftNode, uint256 segmentStartPosition, uint64 eliminableAt)
func (_ITournament *ITournamentFilterer) FilterMatchAdvanced(opts *bind.FilterOpts, matchIdHash [][32]byte) (*ITournamentMatchAdvancedIterator, error) {

	var matchIdHashRule []interface{}
	for _, matchIdHashItem := range matchIdHash {
		matchIdHashRule = append(matchIdHashRule, matchIdHashItem)
	}

	logs, sub, err := _ITournament.contract.FilterLogs(opts, "MatchAdvanced", matchIdHashRule)
	if err != nil {
		return nil, err
	}
	return &ITournamentMatchAdvancedIterator{contract: _ITournament.contract, event: "MatchAdvanced", logs: logs, sub: sub}, nil
}

// WatchMatchAdvanced is a free log subscription operation binding the contract event 0xb6135d4f6f9687ce2f55ce0fd69c7f9d77e003a036ac8e402828abdd973621b1.
//
// Solidity: event MatchAdvanced(bytes32 indexed matchIdHash, bytes32 otherParent, bytes32 leftNode, uint256 segmentStartPosition, uint64 eliminableAt)
func (_ITournament *ITournamentFilterer) WatchMatchAdvanced(opts *bind.WatchOpts, sink chan<- *ITournamentMatchAdvanced, matchIdHash [][32]byte) (event.Subscription, error) {

	var matchIdHashRule []interface{}
	for _, matchIdHashItem := range matchIdHash {
		matchIdHashRule = append(matchIdHashRule, matchIdHashItem)
	}

	logs, sub, err := _ITournament.contract.WatchLogs(opts, "MatchAdvanced", matchIdHashRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ITournamentMatchAdvanced)
				if err := _ITournament.contract.UnpackLog(event, "MatchAdvanced", log); err != nil {
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

// ParseMatchAdvanced is a log parse operation binding the contract event 0xb6135d4f6f9687ce2f55ce0fd69c7f9d77e003a036ac8e402828abdd973621b1.
//
// Solidity: event MatchAdvanced(bytes32 indexed matchIdHash, bytes32 otherParent, bytes32 leftNode, uint256 segmentStartPosition, uint64 eliminableAt)
func (_ITournament *ITournamentFilterer) ParseMatchAdvanced(log types.Log) (*ITournamentMatchAdvanced, error) {
	event := new(ITournamentMatchAdvanced)
	if err := _ITournament.contract.UnpackLog(event, "MatchAdvanced", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ITournamentMatchCreatedIterator is returned from FilterMatchCreated and is used to iterate over the raw logs and unpacked data for MatchCreated events raised by the ITournament contract.
type ITournamentMatchCreatedIterator struct {
	Event *ITournamentMatchCreated // Event containing the contract specifics and raw log

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
func (it *ITournamentMatchCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ITournamentMatchCreated)
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
		it.Event = new(ITournamentMatchCreated)
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
func (it *ITournamentMatchCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ITournamentMatchCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ITournamentMatchCreated represents a MatchCreated event raised by the ITournament contract.
type ITournamentMatchCreated struct {
	MatchIdHash  [32]byte
	One          [32]byte
	Two          [32]byte
	LeftOfTwo    [32]byte
	EliminableAt uint64
	Raw          types.Log // Blockchain specific contextual infos
}

// FilterMatchCreated is a free log retrieval operation binding the contract event 0x6257204c2340779874d2f4e6f94db385ead25b228b1660535f7cbc81b492ac1e.
//
// Solidity: event MatchCreated(bytes32 indexed matchIdHash, bytes32 indexed one, bytes32 indexed two, bytes32 leftOfTwo, uint64 eliminableAt)
func (_ITournament *ITournamentFilterer) FilterMatchCreated(opts *bind.FilterOpts, matchIdHash [][32]byte, one [][32]byte, two [][32]byte) (*ITournamentMatchCreatedIterator, error) {

	var matchIdHashRule []interface{}
	for _, matchIdHashItem := range matchIdHash {
		matchIdHashRule = append(matchIdHashRule, matchIdHashItem)
	}
	var oneRule []interface{}
	for _, oneItem := range one {
		oneRule = append(oneRule, oneItem)
	}
	var twoRule []interface{}
	for _, twoItem := range two {
		twoRule = append(twoRule, twoItem)
	}

	logs, sub, err := _ITournament.contract.FilterLogs(opts, "MatchCreated", matchIdHashRule, oneRule, twoRule)
	if err != nil {
		return nil, err
	}
	return &ITournamentMatchCreatedIterator{contract: _ITournament.contract, event: "MatchCreated", logs: logs, sub: sub}, nil
}

// WatchMatchCreated is a free log subscription operation binding the contract event 0x6257204c2340779874d2f4e6f94db385ead25b228b1660535f7cbc81b492ac1e.
//
// Solidity: event MatchCreated(bytes32 indexed matchIdHash, bytes32 indexed one, bytes32 indexed two, bytes32 leftOfTwo, uint64 eliminableAt)
func (_ITournament *ITournamentFilterer) WatchMatchCreated(opts *bind.WatchOpts, sink chan<- *ITournamentMatchCreated, matchIdHash [][32]byte, one [][32]byte, two [][32]byte) (event.Subscription, error) {

	var matchIdHashRule []interface{}
	for _, matchIdHashItem := range matchIdHash {
		matchIdHashRule = append(matchIdHashRule, matchIdHashItem)
	}
	var oneRule []interface{}
	for _, oneItem := range one {
		oneRule = append(oneRule, oneItem)
	}
	var twoRule []interface{}
	for _, twoItem := range two {
		twoRule = append(twoRule, twoItem)
	}

	logs, sub, err := _ITournament.contract.WatchLogs(opts, "MatchCreated", matchIdHashRule, oneRule, twoRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ITournamentMatchCreated)
				if err := _ITournament.contract.UnpackLog(event, "MatchCreated", log); err != nil {
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

// ParseMatchCreated is a log parse operation binding the contract event 0x6257204c2340779874d2f4e6f94db385ead25b228b1660535f7cbc81b492ac1e.
//
// Solidity: event MatchCreated(bytes32 indexed matchIdHash, bytes32 indexed one, bytes32 indexed two, bytes32 leftOfTwo, uint64 eliminableAt)
func (_ITournament *ITournamentFilterer) ParseMatchCreated(log types.Log) (*ITournamentMatchCreated, error) {
	event := new(ITournamentMatchCreated)
	if err := _ITournament.contract.UnpackLog(event, "MatchCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ITournamentMatchDeletedIterator is returned from FilterMatchDeleted and is used to iterate over the raw logs and unpacked data for MatchDeleted events raised by the ITournament contract.
type ITournamentMatchDeletedIterator struct {
	Event *ITournamentMatchDeleted // Event containing the contract specifics and raw log

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
func (it *ITournamentMatchDeletedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ITournamentMatchDeleted)
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
		it.Event = new(ITournamentMatchDeleted)
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
func (it *ITournamentMatchDeletedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ITournamentMatchDeletedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ITournamentMatchDeleted represents a MatchDeleted event raised by the ITournament contract.
type ITournamentMatchDeleted struct {
	MatchIdHash      [32]byte
	One              [32]byte
	Two              [32]byte
	Reason           uint8
	WinnerCommitment uint8
	Raw              types.Log // Blockchain specific contextual infos
}

// FilterMatchDeleted is a free log retrieval operation binding the contract event 0x1d3ae066e466a0203dcc05b06daeba9922750bcf99aa7837a92123fca1287406.
//
// Solidity: event MatchDeleted(bytes32 indexed matchIdHash, bytes32 indexed one, bytes32 indexed two, uint8 reason, uint8 winnerCommitment)
func (_ITournament *ITournamentFilterer) FilterMatchDeleted(opts *bind.FilterOpts, matchIdHash [][32]byte, one [][32]byte, two [][32]byte) (*ITournamentMatchDeletedIterator, error) {

	var matchIdHashRule []interface{}
	for _, matchIdHashItem := range matchIdHash {
		matchIdHashRule = append(matchIdHashRule, matchIdHashItem)
	}
	var oneRule []interface{}
	for _, oneItem := range one {
		oneRule = append(oneRule, oneItem)
	}
	var twoRule []interface{}
	for _, twoItem := range two {
		twoRule = append(twoRule, twoItem)
	}

	logs, sub, err := _ITournament.contract.FilterLogs(opts, "MatchDeleted", matchIdHashRule, oneRule, twoRule)
	if err != nil {
		return nil, err
	}
	return &ITournamentMatchDeletedIterator{contract: _ITournament.contract, event: "MatchDeleted", logs: logs, sub: sub}, nil
}

// WatchMatchDeleted is a free log subscription operation binding the contract event 0x1d3ae066e466a0203dcc05b06daeba9922750bcf99aa7837a92123fca1287406.
//
// Solidity: event MatchDeleted(bytes32 indexed matchIdHash, bytes32 indexed one, bytes32 indexed two, uint8 reason, uint8 winnerCommitment)
func (_ITournament *ITournamentFilterer) WatchMatchDeleted(opts *bind.WatchOpts, sink chan<- *ITournamentMatchDeleted, matchIdHash [][32]byte, one [][32]byte, two [][32]byte) (event.Subscription, error) {

	var matchIdHashRule []interface{}
	for _, matchIdHashItem := range matchIdHash {
		matchIdHashRule = append(matchIdHashRule, matchIdHashItem)
	}
	var oneRule []interface{}
	for _, oneItem := range one {
		oneRule = append(oneRule, oneItem)
	}
	var twoRule []interface{}
	for _, twoItem := range two {
		twoRule = append(twoRule, twoItem)
	}

	logs, sub, err := _ITournament.contract.WatchLogs(opts, "MatchDeleted", matchIdHashRule, oneRule, twoRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ITournamentMatchDeleted)
				if err := _ITournament.contract.UnpackLog(event, "MatchDeleted", log); err != nil {
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

// ParseMatchDeleted is a log parse operation binding the contract event 0x1d3ae066e466a0203dcc05b06daeba9922750bcf99aa7837a92123fca1287406.
//
// Solidity: event MatchDeleted(bytes32 indexed matchIdHash, bytes32 indexed one, bytes32 indexed two, uint8 reason, uint8 winnerCommitment)
func (_ITournament *ITournamentFilterer) ParseMatchDeleted(log types.Log) (*ITournamentMatchDeleted, error) {
	event := new(ITournamentMatchDeleted)
	if err := _ITournament.contract.UnpackLog(event, "MatchDeleted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ITournamentNewInnerTournamentIterator is returned from FilterNewInnerTournament and is used to iterate over the raw logs and unpacked data for NewInnerTournament events raised by the ITournament contract.
type ITournamentNewInnerTournamentIterator struct {
	Event *ITournamentNewInnerTournament // Event containing the contract specifics and raw log

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
func (it *ITournamentNewInnerTournamentIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ITournamentNewInnerTournament)
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
		it.Event = new(ITournamentNewInnerTournament)
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
func (it *ITournamentNewInnerTournamentIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ITournamentNewInnerTournamentIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ITournamentNewInnerTournament represents a NewInnerTournament event raised by the ITournament contract.
type ITournamentNewInnerTournament struct {
	MatchIdHash     [32]byte
	ChildTournament common.Address
	Raw             types.Log // Blockchain specific contextual infos
}

// FilterNewInnerTournament is a free log retrieval operation binding the contract event 0xc7912008857e6a935ca95124a47677526500edc470687f5ced6e7ad3ca465138.
//
// Solidity: event NewInnerTournament(bytes32 indexed matchIdHash, address indexed childTournament)
func (_ITournament *ITournamentFilterer) FilterNewInnerTournament(opts *bind.FilterOpts, matchIdHash [][32]byte, childTournament []common.Address) (*ITournamentNewInnerTournamentIterator, error) {

	var matchIdHashRule []interface{}
	for _, matchIdHashItem := range matchIdHash {
		matchIdHashRule = append(matchIdHashRule, matchIdHashItem)
	}
	var childTournamentRule []interface{}
	for _, childTournamentItem := range childTournament {
		childTournamentRule = append(childTournamentRule, childTournamentItem)
	}

	logs, sub, err := _ITournament.contract.FilterLogs(opts, "NewInnerTournament", matchIdHashRule, childTournamentRule)
	if err != nil {
		return nil, err
	}
	return &ITournamentNewInnerTournamentIterator{contract: _ITournament.contract, event: "NewInnerTournament", logs: logs, sub: sub}, nil
}

// WatchNewInnerTournament is a free log subscription operation binding the contract event 0xc7912008857e6a935ca95124a47677526500edc470687f5ced6e7ad3ca465138.
//
// Solidity: event NewInnerTournament(bytes32 indexed matchIdHash, address indexed childTournament)
func (_ITournament *ITournamentFilterer) WatchNewInnerTournament(opts *bind.WatchOpts, sink chan<- *ITournamentNewInnerTournament, matchIdHash [][32]byte, childTournament []common.Address) (event.Subscription, error) {

	var matchIdHashRule []interface{}
	for _, matchIdHashItem := range matchIdHash {
		matchIdHashRule = append(matchIdHashRule, matchIdHashItem)
	}
	var childTournamentRule []interface{}
	for _, childTournamentItem := range childTournament {
		childTournamentRule = append(childTournamentRule, childTournamentItem)
	}

	logs, sub, err := _ITournament.contract.WatchLogs(opts, "NewInnerTournament", matchIdHashRule, childTournamentRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ITournamentNewInnerTournament)
				if err := _ITournament.contract.UnpackLog(event, "NewInnerTournament", log); err != nil {
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

// ParseNewInnerTournament is a log parse operation binding the contract event 0xc7912008857e6a935ca95124a47677526500edc470687f5ced6e7ad3ca465138.
//
// Solidity: event NewInnerTournament(bytes32 indexed matchIdHash, address indexed childTournament)
func (_ITournament *ITournamentFilterer) ParseNewInnerTournament(log types.Log) (*ITournamentNewInnerTournament, error) {
	event := new(ITournamentNewInnerTournament)
	if err := _ITournament.contract.UnpackLog(event, "NewInnerTournament", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ITournamentPartialBondRefundIterator is returned from FilterPartialBondRefund and is used to iterate over the raw logs and unpacked data for PartialBondRefund events raised by the ITournament contract.
type ITournamentPartialBondRefundIterator struct {
	Event *ITournamentPartialBondRefund // Event containing the contract specifics and raw log

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
func (it *ITournamentPartialBondRefundIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ITournamentPartialBondRefund)
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
		it.Event = new(ITournamentPartialBondRefund)
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
func (it *ITournamentPartialBondRefundIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ITournamentPartialBondRefundIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ITournamentPartialBondRefund represents a PartialBondRefund event raised by the ITournament contract.
type ITournamentPartialBondRefund struct {
	Recipient common.Address
	Value     *big.Int
	Success   bool
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterPartialBondRefund is a free log retrieval operation binding the contract event 0xeff3c62ee3695d2aab88b472c281a81d4f20bd5e8b13b775210e77a228d46926.
//
// Solidity: event PartialBondRefund(address indexed recipient, uint256 value, bool indexed success)
func (_ITournament *ITournamentFilterer) FilterPartialBondRefund(opts *bind.FilterOpts, recipient []common.Address, success []bool) (*ITournamentPartialBondRefundIterator, error) {

	var recipientRule []interface{}
	for _, recipientItem := range recipient {
		recipientRule = append(recipientRule, recipientItem)
	}

	var successRule []interface{}
	for _, successItem := range success {
		successRule = append(successRule, successItem)
	}

	logs, sub, err := _ITournament.contract.FilterLogs(opts, "PartialBondRefund", recipientRule, successRule)
	if err != nil {
		return nil, err
	}
	return &ITournamentPartialBondRefundIterator{contract: _ITournament.contract, event: "PartialBondRefund", logs: logs, sub: sub}, nil
}

// WatchPartialBondRefund is a free log subscription operation binding the contract event 0xeff3c62ee3695d2aab88b472c281a81d4f20bd5e8b13b775210e77a228d46926.
//
// Solidity: event PartialBondRefund(address indexed recipient, uint256 value, bool indexed success)
func (_ITournament *ITournamentFilterer) WatchPartialBondRefund(opts *bind.WatchOpts, sink chan<- *ITournamentPartialBondRefund, recipient []common.Address, success []bool) (event.Subscription, error) {

	var recipientRule []interface{}
	for _, recipientItem := range recipient {
		recipientRule = append(recipientRule, recipientItem)
	}

	var successRule []interface{}
	for _, successItem := range success {
		successRule = append(successRule, successItem)
	}

	logs, sub, err := _ITournament.contract.WatchLogs(opts, "PartialBondRefund", recipientRule, successRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ITournamentPartialBondRefund)
				if err := _ITournament.contract.UnpackLog(event, "PartialBondRefund", log); err != nil {
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

// ParsePartialBondRefund is a log parse operation binding the contract event 0xeff3c62ee3695d2aab88b472c281a81d4f20bd5e8b13b775210e77a228d46926.
//
// Solidity: event PartialBondRefund(address indexed recipient, uint256 value, bool indexed success)
func (_ITournament *ITournamentFilterer) ParsePartialBondRefund(log types.Log) (*ITournamentPartialBondRefund, error) {
	event := new(ITournamentPartialBondRefund)
	if err := _ITournament.contract.UnpackLog(event, "PartialBondRefund", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
