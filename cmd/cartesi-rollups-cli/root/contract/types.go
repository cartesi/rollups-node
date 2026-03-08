// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import "encoding/json"

// AppResult is the JSON output of "contract app".
type AppResult struct {
	Address          string `json:"address"`
	Owner            string `json:"owner"`
	TemplateHash     string `json:"template_hash"`
	DeploymentBlock  uint64 `json:"deployment_block"`
	ExecutedOutputs  uint64 `json:"executed_outputs"`
	ConsensusAddress string `json:"consensus_address"`
	ConsensusType    string `json:"consensus_type"`
	DataAvailability string `json:"data_availability"`
}

// AuthorityConsensusResult is the JSON output for Authority consensus.
type AuthorityConsensusResult struct {
	Type            string `json:"type"`
	Address         string `json:"address"`
	Owner           string `json:"owner"`
	EpochLength     uint64 `json:"epoch_length"`
	AcceptedClaims  uint64 `json:"accepted_claims"`
	ContractVersion string `json:"contract_version"`
}

// QuorumConsensusResult is the JSON output for Quorum consensus.
type QuorumConsensusResult struct {
	Type            string   `json:"type"`
	Address         string   `json:"address"`
	NumValidators   uint64   `json:"num_validators"`
	QuorumThreshold uint64   `json:"quorum_threshold"`
	Validators      []string `json:"validators"`
	EpochLength     uint64   `json:"epoch_length"`
	AcceptedClaims  uint64   `json:"accepted_claims"`
}

// DaveConsensusResult is the JSON output for DaveConsensus.
// CurrentEpochNumber is the index of the currently sealed epoch (the one with an active
// tournament). Epochs 0..CurrentEpochNumber-1 have been settled; epoch CurrentEpochNumber
// is sealed but not yet settled.
type DaveConsensusResult struct {
	Type               string `json:"type"`
	Address            string `json:"address"`
	InputBox           string `json:"inputbox"`
	Factory            string `json:"factory"`
	DeploymentBlock    uint64 `json:"deployment_block"`
	IsFinished         bool   `json:"is_finished"`
	HasWinner          *bool  `json:"has_winner,omitempty"`
	WinnerCommitment   string `json:"winner_commitment,omitempty"`
	CurrentEpochNumber uint64 `json:"current_epoch_number"`
	InputLowerBound    uint64 `json:"input_lower_bound"`
	InputUpperBound    uint64 `json:"input_upper_bound"`
	RootTournament     string `json:"root_tournament"`
}

// InputBoxResult is the JSON output for InputBox state.
type InputBoxResult struct {
	Address     string `json:"address,omitempty"`
	TotalInputs uint64 `json:"total_inputs"`
}

// CommitmentEvent is a commitment joined event.
type CommitmentEvent struct {
	Commitment     string `json:"commitment"`
	FinalStateHash string `json:"final_state_hash"`
	Submitter      string `json:"submitter"`
	BlockNumber    uint64 `json:"block_number"`
	TxHash         string `json:"tx_hash"`
}

// MatchEvent is a match created event.
type MatchEvent struct {
	MatchIDHash    string  `json:"match_id_hash"`
	CommitmentOne  string  `json:"commitment_one"`
	CommitmentTwo  string  `json:"commitment_two"`
	PlayerOneAddr  string  `json:"player_one_addr,omitempty"`
	PlayerTwoAddr  string  `json:"player_two_addr,omitempty"`
	LeftOfTwo      string  `json:"left_of_two"`
	BlockNumber    uint64  `json:"block_number"`
	TxHash         string  `json:"tx_hash"`
	DeletionReason string  `json:"deletion_reason,omitempty"`
	Winner         string  `json:"winner,omitempty"`
	WinnerAddr     string  `json:"winner_addr,omitempty"`
	DeletionBlock  *uint64 `json:"deletion_block,omitempty"`
	DeletionTxHash string  `json:"deletion_tx_hash,omitempty"`
}

// MatchAdvanceEvent is a match advance (bisection step) event.
type MatchAdvanceEvent struct {
	MatchIDHash string `json:"match_id_hash"`
	OtherParent string `json:"other_parent"`
	LeftNode    string `json:"left_node"`
	BlockNumber uint64 `json:"block_number"`
	TxHash      string `json:"tx_hash"`
}

// TournamentResult is the JSON output for a tournament.
type TournamentResult struct {
	Address           string              `json:"address"`
	Level             uint64              `json:"level"`
	MaxLevel          uint64              `json:"max_level"`
	Log2Step          uint64              `json:"log2step"`
	Height            uint64              `json:"height"`
	Closed            bool                `json:"closed"`
	Finished          bool                `json:"finished"`
	FinishedAtBlock   *uint64             `json:"finished_at_block,omitempty"`
	HasWinner         *bool               `json:"has_winner,omitempty"`
	WinnerCommitment  string              `json:"winner_commitment,omitempty"`
	WinnerAddress     string              `json:"winner_address,omitempty"`
	FinalMachineHash  string              `json:"final_machine_hash,omitempty"`
	BondWei           string              `json:"bond_wei"`
	BondETH           string              `json:"bond_eth"`
	CommitmentsJoined uint64              `json:"commitments_joined"`
	MatchesCreated    uint64              `json:"matches_created"`
	MatchesAdvanced   uint64              `json:"matches_advanced"`
	MatchesDeleted    uint64              `json:"matches_deleted"`
	InnerTournaments  uint64              `json:"inner_tournaments"`
	CanBeEliminated   *bool               `json:"can_be_eliminated,omitempty"`
	Commitments       []CommitmentEvent   `json:"commitments,omitempty"`
	Matches           []MatchEvent        `json:"matches,omitempty"`
	Advances          []MatchAdvanceEvent `json:"advances,omitempty"`
	Children          []*TournamentResult `json:"children,omitempty"`
}

// ClaimEvent is a claim event in the epoch history.
type ClaimEvent struct {
	EventType          string  `json:"event_type"`
	BlockNumber        uint64  `json:"block_number"`
	TxHash             string  `json:"tx_hash"`
	Submitter          string  `json:"submitter,omitempty"`
	LastProcessedBlock uint64  `json:"last_processed_block"`
	OutputsMerkleRoot  string  `json:"outputs_merkle_root"`
	OutputsRootValid   *bool   `json:"outputs_root_valid,omitempty"`
	VotesForClaim      *uint64 `json:"votes_for_claim,omitempty"`
	VotesNeeded        *uint64 `json:"votes_needed,omitempty"`
}

// EpochEvent is a single sealed epoch event (DaveConsensus).
type EpochEvent struct {
	EpochNumber        uint64 `json:"epoch_number"`
	BlockNumber        uint64 `json:"block_number"`
	TxHash             string `json:"tx_hash"`
	InputLowerBound    uint64 `json:"input_lower_bound"`
	InputUpperBound    uint64 `json:"input_upper_bound"`
	InitialMachineHash string `json:"initial_machine_hash"`
	OutputsMerkleRoot  string `json:"outputs_merkle_root"`
	OutputsRootValid   *bool  `json:"outputs_root_valid,omitempty"`
	Tournament         string `json:"tournament"`
}

// EpochHistory is the full epoch history output.
type EpochHistory struct {
	ConsensusType string       `json:"consensus_type"`
	TemplateHash  string       `json:"template_hash,omitempty"`
	Epochs        []EpochEvent `json:"epochs,omitempty"`
	Claims        []ClaimEvent `json:"claims,omitempty"`
}

// SummaryResult is the JSON output of "contract summary".
type SummaryResult struct {
	Application     *AppResult        `json:"application,omitempty"`
	AppError        string            `json:"app_error,omitempty"`
	Consensus       json.RawMessage   `json:"consensus,omitempty"`
	ConsensusError  string            `json:"consensus_error,omitempty"`
	InputBox        *InputBoxResult   `json:"inputbox,omitempty"`
	InputBoxError   string            `json:"inputbox_error,omitempty"`
	RootTournament  *TournamentResult `json:"root_tournament,omitempty"`
	TournamentError string            `json:"tournament_error,omitempty"`
}

// OutputResult is the JSON output for a single output execution check.
type OutputResult struct {
	OutputIndex uint64 `json:"output_index"`
	Executed    bool   `json:"executed"`
}

// OutputBatchResult is the JSON output for batch output execution status.
type OutputBatchResult struct {
	TotalExecuted uint64         `json:"total_executed"`
	Outputs       []OutputResult `json:"outputs,omitempty"`
}

// CommitmentResult is the JSON output for a commitment's on-chain state.
type CommitmentResult struct {
	Commitment       string `json:"commitment"`
	Tournament       string `json:"tournament"`
	TournamentLevel  string `json:"tournament_level"`
	ClockAllowance   uint64 `json:"clock_allowance"`
	ClockStartBlock  uint64 `json:"clock_start_block"`
	FinalMachineHash string `json:"final_machine_hash"`
}

// MatchResult is the JSON output for a match's bisection state.
type MatchResult struct {
	MatchIDHash         string `json:"match_id_hash"`
	Tournament          string `json:"tournament"`
	CommitmentOne       string `json:"commitment_one"`
	CommitmentTwo       string `json:"commitment_two"`
	PlayerOneAddr       string `json:"player_one_addr,omitempty"`
	PlayerTwoAddr       string `json:"player_two_addr,omitempty"`
	CurrentHeight       uint64 `json:"current_height"`
	RunningLeafPosition string `json:"running_leaf_position"`
	MachineCycle        string `json:"machine_cycle"`
	CanWinByTimeout     bool   `json:"can_win_by_timeout"`
	LeftNode            string `json:"left_node"`
	RightNode           string `json:"right_node"`
	OtherParent         string `json:"other_parent"`
}
