// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

type TournamentKind string

const (
	TournamentKindLeaf    TournamentKind = "LEAF"
	TournamentKindNonLeaf TournamentKind = "NON_LEAF"
)

var TournamentKindAllValues = []TournamentKind{
	TournamentKindLeaf,
	TournamentKindNonLeaf,
}

func (e *TournamentKind) Scan(value any) error {
	return scanEnum(e, value, TournamentKindAllValues, "TournamentKind")
}

func (e TournamentKind) String() string {
	return string(e)
}

type TournamentStandingState string

const (
	TournamentStandingMatchesActive                TournamentStandingState = "MATCHES_ACTIVE"
	TournamentStandingAwaitingClosure              TournamentStandingState = "AWAITING_CLOSURE"
	TournamentStandingRootWinner                   TournamentStandingState = "ROOT_WINNER"
	TournamentStandingRootFailed                   TournamentStandingState = "ROOT_FAILED"
	TournamentStandingInnerWinner                  TournamentStandingState = "INNER_WINNER"
	TournamentStandingInnerEliminableNoWinner      TournamentStandingState = "INNER_ELIMINABLE_NO_WINNER"
	TournamentStandingInnerEliminableWinnerExpired TournamentStandingState = "INNER_ELIMINABLE_WINNER_EXPIRED"
)

var TournamentStandingStateAllValues = []TournamentStandingState{
	TournamentStandingMatchesActive,
	TournamentStandingAwaitingClosure,
	TournamentStandingRootWinner,
	TournamentStandingRootFailed,
	TournamentStandingInnerWinner,
	TournamentStandingInnerEliminableNoWinner,
	TournamentStandingInnerEliminableWinnerExpired,
}

func (e *TournamentStandingState) Scan(value any) error {
	return scanEnum(e, value, TournamentStandingStateAllValues, "TournamentStandingState")
}

func (e TournamentStandingState) String() string {
	return string(e)
}

type MatchPhase string

const (
	MatchPhaseUninitialized MatchPhase = "UNINITIALIZED"
	MatchPhaseBisecting     MatchPhase = "BISECTING"
	MatchPhaseReadyToSeal   MatchPhase = "READY_TO_SEAL"
	MatchPhaseSealed        MatchPhase = "SEALED"
)

var MatchPhaseAllValues = []MatchPhase{
	MatchPhaseUninitialized,
	MatchPhaseBisecting,
	MatchPhaseReadyToSeal,
	MatchPhaseSealed,
}

func (e *MatchPhase) Scan(value any) error {
	return scanEnum(e, value, MatchPhaseAllValues, "MatchPhase")
}

func (e MatchPhase) String() string {
	return string(e)
}

type CommitmentSide string

const (
	CommitmentSideOne CommitmentSide = "ONE"
	CommitmentSideTwo CommitmentSide = "TWO"
)

var CommitmentSideAllValues = []CommitmentSide{
	CommitmentSideOne,
	CommitmentSideTwo,
}

func (e *CommitmentSide) Scan(value any) error {
	return scanEnum(e, value, CommitmentSideAllValues, "CommitmentSide")
}

func (e CommitmentSide) String() string {
	return string(e)
}

type MatchTimeoutOutcome string

const (
	MatchTimeoutNone          MatchTimeoutOutcome = "NONE"
	MatchTimeoutOneWins       MatchTimeoutOutcome = "ONE_WINS"
	MatchTimeoutTwoWins       MatchTimeoutOutcome = "TWO_WINS"
	MatchTimeoutEliminateBoth MatchTimeoutOutcome = "ELIMINATE_BOTH"
)

var MatchTimeoutOutcomeAllValues = []MatchTimeoutOutcome{
	MatchTimeoutNone,
	MatchTimeoutOneWins,
	MatchTimeoutTwoWins,
	MatchTimeoutEliminateBoth,
}

func (e *MatchTimeoutOutcome) Scan(value any) error {
	return scanEnum(e, value, MatchTimeoutOutcomeAllValues, "MatchTimeoutOutcome")
}

func (e MatchTimeoutOutcome) String() string {
	return string(e)
}

type InnerTournamentDisposition string

const (
	InnerTournamentUnsettled  InnerTournamentDisposition = "UNSETTLED"
	InnerTournamentWinner     InnerTournamentDisposition = "WINNER"
	InnerTournamentEliminable InnerTournamentDisposition = "ELIMINABLE"
)

var InnerTournamentDispositionAllValues = []InnerTournamentDisposition{
	InnerTournamentUnsettled,
	InnerTournamentWinner,
	InnerTournamentEliminable,
}

func (e *InnerTournamentDisposition) Scan(value any) error {
	return scanEnum(e, value, InnerTournamentDispositionAllValues, "InnerTournamentDisposition")
}

func (e InnerTournamentDisposition) String() string {
	return string(e)
}

type BondDisposition string

const (
	BondDispositionTournamentRunning BondDisposition = "TOURNAMENT_RUNNING"
	BondDispositionNoWinner          BondDisposition = "NO_WINNER"
	BondDispositionRecoverable       BondDisposition = "RECOVERABLE"
	BondDispositionRecovered         BondDisposition = "RECOVERED"
)

var BondDispositionAllValues = []BondDisposition{
	BondDispositionTournamentRunning,
	BondDispositionNoWinner,
	BondDispositionRecoverable,
	BondDispositionRecovered,
}

func (e *BondDisposition) Scan(value any) error {
	return scanEnum(e, value, BondDispositionAllValues, "BondDisposition")
}

func (e BondDisposition) String() string {
	return string(e)
}

type BondEventType string

const (
	BondEventPartialRefund BondEventType = "PARTIAL_BOND_REFUND"
	BondEventRecovered     BondEventType = "BOND_RECOVERED"
)

var BondEventTypeAllValues = []BondEventType{
	BondEventPartialRefund,
	BondEventRecovered,
}

func (e *BondEventType) Scan(value any) error {
	return scanEnum(e, value, BondEventTypeAllValues, "BondEventType")
}

func (e BondEventType) String() string {
	return string(e)
}
