// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"encoding/json"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type TournamentSnapshot struct {
	AsOfBlock        uint64                  `json:"as_of_block"`
	Standing         TournamentStandingState `json:"standing"`
	AcceptsJoins     bool                    `json:"accepts_joins"`
	Candidate        *common.Hash            `json:"candidate"`
	WinnerCommitment *common.Hash            `json:"winner_commitment"`
	FinalStateHash   *common.Hash            `json:"final_state_hash"`
	ParentCommitment *common.Hash            `json:"parent_commitment"`
	FinishedAtBlock  uint64                  `json:"finished_at_block"`
	WinnerExpiresAt  uint64                  `json:"winner_expires_at"`
	InnerResult      *TournamentInnerResult  `json:"inner_result"`
	BondRecovery     TournamentBondRecovery  `json:"bond_recovery"`
}

func (value TournamentSnapshot) MarshalJSON() ([]byte, error) {
	type Alias TournamentSnapshot
	return json.Marshal(struct {
		*Alias
		AsOfBlock       hexutil.Uint64 `json:"as_of_block"`
		FinishedAtBlock hexutil.Uint64 `json:"finished_at_block"`
		WinnerExpiresAt hexutil.Uint64 `json:"winner_expires_at"`
	}{
		Alias:           (*Alias)(&value),
		AsOfBlock:       hexutil.Uint64(value.AsOfBlock),
		FinishedAtBlock: hexutil.Uint64(value.FinishedAtBlock),
		WinnerExpiresAt: hexutil.Uint64(value.WinnerExpiresAt),
	})
}

func (value *TournamentSnapshot) UnmarshalJSON(data []byte) error {
	type Alias TournamentSnapshot
	var decoded TournamentSnapshot
	aux := struct {
		*Alias
		AsOfBlock       hexutil.Uint64 `json:"as_of_block"`
		FinishedAtBlock hexutil.Uint64 `json:"finished_at_block"`
		WinnerExpiresAt hexutil.Uint64 `json:"winner_expires_at"`
	}{Alias: (*Alias)(&decoded)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	decoded.AsOfBlock = uint64(aux.AsOfBlock)
	decoded.FinishedAtBlock = uint64(aux.FinishedAtBlock)
	decoded.WinnerExpiresAt = uint64(aux.WinnerExpiresAt)
	*value = decoded
	return nil
}

type TournamentInnerResult struct {
	Disposition      InnerTournamentDisposition `json:"disposition"`
	ParentCommitment *common.Hash               `json:"parent_commitment"`
	PausedAllowance  uint64                     `json:"paused_allowance"`
}

func (value TournamentInnerResult) MarshalJSON() ([]byte, error) {
	type Alias TournamentInnerResult
	return json.Marshal(struct {
		*Alias
		PausedAllowance hexutil.Uint64 `json:"paused_allowance"`
	}{
		Alias:           (*Alias)(&value),
		PausedAllowance: hexutil.Uint64(value.PausedAllowance),
	})
}

func (value *TournamentInnerResult) UnmarshalJSON(data []byte) error {
	type Alias TournamentInnerResult
	var decoded TournamentInnerResult
	aux := struct {
		*Alias
		PausedAllowance hexutil.Uint64 `json:"paused_allowance"`
	}{Alias: (*Alias)(&decoded)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	decoded.PausedAllowance = uint64(aux.PausedAllowance)
	*value = decoded
	return nil
}

type TournamentBondRecovery struct {
	Disposition BondDisposition `json:"disposition"`
	Claimer     *common.Address `json:"claimer"`
	Payment     *Uint256        `json:"payment"`
}

type CommitmentSnapshot struct {
	AsOfBlock      uint64         `json:"as_of_block"`
	Claimer        common.Address `json:"claimer"`
	ClockRunning   bool           `json:"clock_running"`
	ClockDeadline  uint64         `json:"clock_deadline"`
	ClockAllowance uint64         `json:"clock_allowance"`
}

func (value CommitmentSnapshot) MarshalJSON() ([]byte, error) {
	type Alias CommitmentSnapshot
	return json.Marshal(struct {
		*Alias
		AsOfBlock      hexutil.Uint64 `json:"as_of_block"`
		ClockDeadline  hexutil.Uint64 `json:"clock_deadline"`
		ClockAllowance hexutil.Uint64 `json:"clock_allowance"`
	}{
		Alias:          (*Alias)(&value),
		AsOfBlock:      hexutil.Uint64(value.AsOfBlock),
		ClockDeadline:  hexutil.Uint64(value.ClockDeadline),
		ClockAllowance: hexutil.Uint64(value.ClockAllowance),
	})
}

func (value *CommitmentSnapshot) UnmarshalJSON(data []byte) error {
	type Alias CommitmentSnapshot
	var decoded CommitmentSnapshot
	aux := struct {
		*Alias
		AsOfBlock      hexutil.Uint64 `json:"as_of_block"`
		ClockDeadline  hexutil.Uint64 `json:"clock_deadline"`
		ClockAllowance hexutil.Uint64 `json:"clock_allowance"`
	}{Alias: (*Alias)(&decoded)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	decoded.AsOfBlock = uint64(aux.AsOfBlock)
	decoded.ClockDeadline = uint64(aux.ClockDeadline)
	decoded.ClockAllowance = uint64(aux.ClockAllowance)
	*value = decoded
	return nil
}

type MatchSnapshot struct {
	AsOfBlock      uint64                  `json:"as_of_block"`
	Phase          MatchPhase              `json:"phase"`
	Bisection      *MatchBisectionSnapshot `json:"bisection"`
	Sealed         *MatchSealedSnapshot    `json:"sealed"`
	TimeoutOutcome MatchTimeoutOutcome     `json:"timeout_outcome"`
	DeferredCharge uint64                  `json:"deferred_charge"`
}

func (value MatchSnapshot) MarshalJSON() ([]byte, error) {
	type Alias MatchSnapshot
	return json.Marshal(struct {
		*Alias
		AsOfBlock      hexutil.Uint64 `json:"as_of_block"`
		DeferredCharge hexutil.Uint64 `json:"deferred_charge"`
	}{
		Alias:          (*Alias)(&value),
		AsOfBlock:      hexutil.Uint64(value.AsOfBlock),
		DeferredCharge: hexutil.Uint64(value.DeferredCharge),
	})
}

func (value *MatchSnapshot) UnmarshalJSON(data []byte) error {
	type Alias MatchSnapshot
	var decoded MatchSnapshot
	aux := struct {
		*Alias
		AsOfBlock      hexutil.Uint64 `json:"as_of_block"`
		DeferredCharge hexutil.Uint64 `json:"deferred_charge"`
	}{Alias: (*Alias)(&decoded)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	decoded.AsOfBlock = uint64(aux.AsOfBlock)
	decoded.DeferredCharge = uint64(aux.DeferredCharge)
	*value = decoded
	return nil
}

type MatchBisectionSnapshot struct {
	RevealingParent      common.Hash    `json:"revealing_parent"`
	WaitingLeft          common.Hash    `json:"waiting_left"`
	WaitingRight         common.Hash    `json:"waiting_right"`
	SegmentStartPosition Uint256        `json:"segment_start_position"`
	SegmentStartCycle    Uint256        `json:"segment_start_cycle"`
	CurrentHeight        *uint64        `json:"current_height"`
	Responder            CommitmentSide `json:"responder"`
}

func (value MatchBisectionSnapshot) MarshalJSON() ([]byte, error) {
	type Alias MatchBisectionSnapshot
	return json.Marshal(struct {
		*Alias
		CurrentHeight *hexutil.Uint64 `json:"current_height"`
	}{
		Alias:         (*Alias)(&value),
		CurrentHeight: (*hexutil.Uint64)(value.CurrentHeight),
	})
}

func (value *MatchBisectionSnapshot) UnmarshalJSON(data []byte) error {
	type Alias MatchBisectionSnapshot
	var decoded MatchBisectionSnapshot
	aux := struct {
		*Alias
		CurrentHeight *hexutil.Uint64 `json:"current_height"`
	}{Alias: (*Alias)(&decoded)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	decoded.CurrentHeight = (*uint64)(aux.CurrentHeight)
	*value = decoded
	return nil
}

type MatchSealedSnapshot struct {
	AgreeState         common.Hash `json:"agree_state"`
	DivergencePosition Uint256     `json:"divergence_position"`
	DivergenceCycle    Uint256     `json:"divergence_cycle"`
	FinalStateOne      common.Hash `json:"final_state_one"`
	FinalStateTwo      common.Hash `json:"final_state_two"`
}

type TournamentCreationEvent struct {
	BlockNumber uint64      `json:"block_number"`
	TxHash      common.Hash `json:"tx_hash"`
	LogIndex    uint64      `json:"log_index"`
}

func (value TournamentCreationEvent) MarshalJSON() ([]byte, error) {
	type Alias TournamentCreationEvent
	return json.Marshal(struct {
		*Alias
		BlockNumber hexutil.Uint64 `json:"block_number"`
		LogIndex    hexutil.Uint64 `json:"log_index"`
	}{
		Alias:       (*Alias)(&value),
		BlockNumber: hexutil.Uint64(value.BlockNumber),
		LogIndex:    hexutil.Uint64(value.LogIndex),
	})
}

func (value *TournamentCreationEvent) UnmarshalJSON(data []byte) error {
	type Alias TournamentCreationEvent
	var decoded TournamentCreationEvent
	aux := struct {
		*Alias
		BlockNumber hexutil.Uint64 `json:"block_number"`
		LogIndex    hexutil.Uint64 `json:"log_index"`
	}{Alias: (*Alias)(&decoded)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	decoded.BlockNumber = uint64(aux.BlockNumber)
	decoded.LogIndex = uint64(aux.LogIndex)
	*value = decoded
	return nil
}

type LeafMatchSeal struct {
	EliminableAt uint64      `json:"eliminable_at"`
	BlockNumber  uint64      `json:"block_number"`
	TxHash       common.Hash `json:"tx_hash"`
	LogIndex     uint64      `json:"log_index"`
}

func (value LeafMatchSeal) MarshalJSON() ([]byte, error) {
	type Alias LeafMatchSeal
	return json.Marshal(struct {
		*Alias
		EliminableAt hexutil.Uint64 `json:"eliminable_at"`
		BlockNumber  hexutil.Uint64 `json:"block_number"`
		LogIndex     hexutil.Uint64 `json:"log_index"`
	}{
		Alias:        (*Alias)(&value),
		EliminableAt: hexutil.Uint64(value.EliminableAt),
		BlockNumber:  hexutil.Uint64(value.BlockNumber),
		LogIndex:     hexutil.Uint64(value.LogIndex),
	})
}

func (value *LeafMatchSeal) UnmarshalJSON(data []byte) error {
	type Alias LeafMatchSeal
	var decoded LeafMatchSeal
	aux := struct {
		*Alias
		EliminableAt hexutil.Uint64 `json:"eliminable_at"`
		BlockNumber  hexutil.Uint64 `json:"block_number"`
		LogIndex     hexutil.Uint64 `json:"log_index"`
	}{Alias: (*Alias)(&decoded)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	decoded.EliminableAt = uint64(aux.EliminableAt)
	decoded.BlockNumber = uint64(aux.BlockNumber)
	decoded.LogIndex = uint64(aux.LogIndex)
	*value = decoded
	return nil
}

type BondEvent struct {
	ApplicationID     int64              `json:"-"`
	EpochIndex        uint64             `json:"epoch_index"`
	TournamentAddress common.Address     `json:"tournament_address"`
	Type              BondEventType      `json:"type"`
	BlockNumber       uint64             `json:"block_number"`
	TxHash            common.Hash        `json:"tx_hash"`
	LogIndex          uint64             `json:"log_index"`
	Refund            *PartialBondRefund `json:"refund"`
	Recovery          *BondRecovered     `json:"recovery"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

func (value BondEvent) MarshalJSON() ([]byte, error) {
	type Alias BondEvent
	return json.Marshal(struct {
		*Alias
		EpochIndex  hexutil.Uint64 `json:"epoch_index"`
		BlockNumber hexutil.Uint64 `json:"block_number"`
		LogIndex    hexutil.Uint64 `json:"log_index"`
	}{
		Alias:       (*Alias)(&value),
		EpochIndex:  hexutil.Uint64(value.EpochIndex),
		BlockNumber: hexutil.Uint64(value.BlockNumber),
		LogIndex:    hexutil.Uint64(value.LogIndex),
	})
}

func (value *BondEvent) UnmarshalJSON(data []byte) error {
	type Alias BondEvent
	var decoded BondEvent
	aux := struct {
		*Alias
		EpochIndex  hexutil.Uint64 `json:"epoch_index"`
		BlockNumber hexutil.Uint64 `json:"block_number"`
		LogIndex    hexutil.Uint64 `json:"log_index"`
	}{Alias: (*Alias)(&decoded)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	decoded.EpochIndex = uint64(aux.EpochIndex)
	decoded.BlockNumber = uint64(aux.BlockNumber)
	decoded.LogIndex = uint64(aux.LogIndex)
	*value = decoded
	return nil
}

type PartialBondRefund struct {
	Recipient common.Address `json:"recipient"`
	Value     Uint256        `json:"value"`
	Success   bool           `json:"success"`
}

type BondRecovered struct {
	Commitment common.Hash    `json:"commitment"`
	Claimer    common.Address `json:"claimer"`
	Payment    Uint256        `json:"payment"`
	Burned     Uint256        `json:"burned"`
}
