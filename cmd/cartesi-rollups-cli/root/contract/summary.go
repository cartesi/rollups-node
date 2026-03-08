// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"os"

	"github.com/cartesi/rollups-node/pkg/contracts/iauthority"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var summaryCmd = &cobra.Command{
	Use:   "summary <application-address>",
	Short: "Full diagnostic snapshot (composes app + consensus + inputbox + root tournament)",
	Args:  cobra.ExactArgs(1),
	RunE:  runSummary,
}

// consensusResult holds the query result for any consensus type.
type consensusResult struct {
	cType  string
	result any // one of *Authority/Quorum/DaveConsensusResult
	err    error
}

func runSummary(cmd *cobra.Command, args []string) error {
	cc, cancel, err := initChainClient(cmd, args)
	if err != nil {
		return err
	}
	defer cancel()
	defer cc.eth.Close()

	// Pre-resolve consensus address and type once. This avoids duplicate RPC calls
	// between the consensus and tournament sections.
	consensusAddr, consensusDetectErr := cc.getConsensusAddress()
	var cType consensusType
	var contractVersion string
	if consensusDetectErr == nil && consensusAddr != (common.Address{}) {
		cType, contractVersion, consensusDetectErr = cc.detectConsensus(consensusAddr)
	}

	// Use plain errgroup (NOT WithContext) for partial failure — if one section fails,
	// the others should continue to completion.
	var g errgroup.Group
	g.SetLimit(4) //nolint:mnd

	// Each goroutine writes to its own dedicated variable. g.Wait() provides
	// the happens-before guarantee, so no mutex is needed.
	var (
		appResult      *AppResult
		appErr         error
		cr             consensusResult
		inputBoxResult *InputBoxResult
		inputBoxErr    error
		tournResult    *TournamentResult
		tournErr       error
	)

	// Section 1: Application.
	g.Go(func() error {
		appResult, appErr = cc.queryApp()
		return nil // always nil — partial failure
	})

	// Section 2: Consensus query (uses pre-resolved address and type).
	g.Go(func() error {
		if consensusDetectErr != nil {
			cr.err = consensusDetectErr
			return nil
		}
		if consensusAddr == (common.Address{}) {
			return nil
		}

		cr.cType = cType.String()

		var cResult any
		var cErr error
		switch cType {
		case consensusAuthority:
			cResult, cErr = cc.queryAuthority(consensusAddr, contractVersion)
		case consensusQuorum:
			cResult, cErr = cc.queryQuorum(consensusAddr)
		case consensusDave:
			cResult, cErr = cc.queryDave(consensusAddr)
		case consensusUnknown:
			cErr = fmt.Errorf("unknown consensus type at %s", consensusAddr)
		}

		cr.result, cr.err = cResult, cErr
		return nil
	})

	// Section 3: InputBox.
	g.Go(func() error {
		inputBoxResult, inputBoxErr = cc.queryInputBox()
		return nil
	})

	// Section 4: Root Tournament (only for DaveConsensus).
	if consensusDetectErr == nil && cType == consensusDave {
		g.Go(func() error {
			daveCaller, dErr := idaveconsensus.NewIDaveConsensusCaller(
				consensusAddr, cc.eth)
			if dErr != nil {
				tournErr = dErr
				return nil
			}

			sealed, dErr := daveCaller.GetCurrentSealedEpoch(cc.callOpts)
			if dErr != nil {
				tournErr = dErr
				return nil
			}
			if sealed.Tournament == (common.Address{}) {
				return nil // no sealed epoch yet
			}

			tournResult, tournErr = cc.queryTournament(sealed.Tournament)
			return nil
		})
	}

	_ = g.Wait() // errors are always nil (partial failure)

	if jsonParam {
		result := &SummaryResult{}
		if appErr != nil {
			result.AppError = appErr.Error()
		} else {
			result.Application = appResult
		}
		if cr.err != nil {
			result.ConsensusError = cr.err.Error()
		} else if cr.result != nil {
			result.Consensus, _ = json.Marshal(cr.result)
		}
		if inputBoxErr != nil {
			result.InputBoxError = inputBoxErr.Error()
		} else {
			result.InputBox = inputBoxResult
		}
		if tournErr != nil {
			result.TournamentError = tournErr.Error()
		} else {
			result.RootTournament = tournResult
		}
		return outputJSON(result)
	}

	// Text output with partial failure.
	p := &printer{w: os.Stdout}

	if appErr != nil {
		p.withSection(fmt.Sprintf("Application  %s", formatAddr(cc.appAddr)), func() {
			p.fieldErr("application query", appErr)
		})
	} else if appResult != nil {
		p.withSection(fmt.Sprintf("Application  %s", appResult.Address), func() {
			p.field("Template Hash", appResult.TemplateHash)
			p.field("Owner", appResult.Owner)
			p.field("Deployment Block", fmt.Sprintf("%d", appResult.DeploymentBlock))
			p.field("Executed Outputs", fmt.Sprintf("%d", appResult.ExecutedOutputs))
			p.field("Consensus",
				fmt.Sprintf("%s (%s)", appResult.ConsensusAddress, appResult.ConsensusType))
			p.field("Data Availability", appResult.DataAvailability)
		})
	}

	displayConsensusAddr := consensusAddr
	if appResult != nil {
		displayConsensusAddr = common.HexToAddress(appResult.ConsensusAddress)
	}

	if cr.err != nil {
		label := "Consensus"
		if cr.cType != "" {
			label = cr.cType
		}
		p.withSection(fmt.Sprintf("%s  %s", label, formatAddr(displayConsensusAddr)), func() {
			p.fieldErr("consensus query", cr.err)
		})
	} else if cr.result != nil {
		printConsensusSummary(p, cr)
	}

	// Root Tournament (DaveConsensus only).
	if tournErr != nil {
		p.withSection("Root Tournament", func() {
			p.fieldErr("tournament query", tournErr)
		})
	} else if tournResult != nil {
		printTournamentBasic(p, tournResult)
	}

	// InputBox.
	if inputBoxErr != nil {
		p.withSection("InputBox", func() {
			p.fieldErr("inputbox query", inputBoxErr)
		})
	} else if inputBoxResult != nil {
		p.withSection("InputBox", func() {
			p.field("Total Inputs", fmt.Sprintf("%d", inputBoxResult.TotalInputs))
		})
	}

	p.footer(cc.blockNum, cc.chainID, cc.resolveTimestamp(cc.blockNum))
	return nil
}

func printConsensusSummary(p *printer, cr consensusResult) {
	switch r := cr.result.(type) {
	case *AuthorityConsensusResult:
		p.withSection(fmt.Sprintf("Authority  %s", r.Address), func() {
			p.field("Owner (Validator)", r.Owner)
			p.field("Epoch Length", fmt.Sprintf("%d blocks", r.EpochLength))
			p.field("Accepted Claims", fmt.Sprintf("%d", r.AcceptedClaims))
			p.field("IConsensus Version", r.ContractVersion)
		})
	case *QuorumConsensusResult:
		p.withSection(fmt.Sprintf("Quorum  %s", r.Address), func() {
			p.field("Validators", fmt.Sprintf("%d", r.NumValidators))
			p.field("Quorum Threshold",
				fmt.Sprintf("%d (computed: strict majority)", r.QuorumThreshold))
			p.field("Epoch Length", fmt.Sprintf("%d blocks", r.EpochLength))
			p.field("Accepted Claims", fmt.Sprintf("%d", r.AcceptedClaims))
		})
	case *DaveConsensusResult:
		p.withSection(fmt.Sprintf("DaveConsensus  %s", r.Address), func() {
			p.field("Deployment Block", fmt.Sprintf("%d", r.DeploymentBlock))
			printTournamentFinished(p, r)
			p.field("Current Sealed Epoch", fmt.Sprintf("%d", r.CurrentEpochNumber))
			p.field("Root Tournament", r.RootTournament)
		})
	}
}

// queryAuthority returns a structured Authority result.
func (c *chainClient) queryAuthority(
	addr common.Address, contractVersion string,
) (*AuthorityConsensusResult, error) {
	if err := c.ensureContract(addr, "Authority"); err != nil {
		return nil, err
	}
	caller, err := iauthority.NewIAuthorityCaller(addr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind IAuthority: %w", err)
	}

	owner, err := caller.Owner(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("IAuthority.Owner: %w", err)
	}

	epochLengthRaw, err := caller.GetEpochLength(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetEpochLength: %w", err)
	}
	epochLength, err := safeUint64(epochLengthRaw, "epoch length")
	if err != nil {
		return nil, err
	}

	claimsRaw, err := caller.GetNumberOfAcceptedClaims(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetNumberOfAcceptedClaims: %w", err)
	}
	claims, err := safeUint64(claimsRaw, "accepted claims")
	if err != nil {
		return nil, err
	}

	return &AuthorityConsensusResult{
		Type:            "Authority",
		Address:         formatAddr(addr),
		Owner:           formatAddr(owner),
		EpochLength:     epochLength,
		AcceptedClaims:  claims,
		ContractVersion: contractVersion,
	}, nil
}

// queryQuorum returns a structured Quorum result.
func (c *chainClient) queryQuorum(
	addr common.Address,
) (*QuorumConsensusResult, error) {
	if err := c.ensureContract(addr, "Quorum"); err != nil {
		return nil, err
	}
	caller, err := iquorum.NewIQuorumCaller(addr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind IQuorum: %w", err)
	}

	epochLengthRaw, err := caller.GetEpochLength(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetEpochLength: %w", err)
	}
	epochLength, err := safeUint64(epochLengthRaw, "epoch length")
	if err != nil {
		return nil, err
	}

	claimsRaw, err := caller.GetNumberOfAcceptedClaims(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetNumberOfAcceptedClaims: %w", err)
	}
	claims, err := safeUint64(claimsRaw, "accepted claims")
	if err != nil {
		return nil, err
	}

	numValRaw, err := caller.NumOfValidators(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("NumOfValidators: %w", err)
	}
	numVal, err := safeUint64(numValRaw, "num validators")
	if err != nil {
		return nil, err
	}

	const maxValidators = 10000
	if numVal > maxValidators {
		slog.Warn("validator count exceeds cap, truncating",
			"reported", numVal, "cap", maxValidators)
		numVal = maxValidators
	}
	validators := make([]string, 0, numVal)
	for i := uint64(1); i <= numVal; i++ {
		valAddr, err := caller.ValidatorById(c.callOpts, new(big.Int).SetUint64(i))
		if err != nil {
			return nil, fmt.Errorf("ValidatorById(%d): %w", i, err)
		}
		validators = append(validators, formatAddr(valAddr))
	}

	threshold := 1 + numVal/2 //nolint:mnd

	return &QuorumConsensusResult{
		Type:            "Quorum",
		Address:         formatAddr(addr),
		NumValidators:   numVal,
		QuorumThreshold: threshold,
		Validators:      validators,
		EpochLength:     epochLength,
		AcceptedClaims:  claims,
	}, nil
}

// queryDave returns a structured DaveConsensus result.
func (c *chainClient) queryDave(
	addr common.Address,
) (*DaveConsensusResult, error) {
	if err := c.ensureContract(addr, "DaveConsensus"); err != nil {
		return nil, err
	}
	caller, err := idaveconsensus.NewIDaveConsensusCaller(addr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind IDaveConsensus: %w", err)
	}

	settleInfo, err := caller.CanSettle(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("CanSettle: %w", err)
	}

	sealed, err := caller.GetCurrentSealedEpoch(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetCurrentSealedEpoch: %w", err)
	}

	epochNumber, err := safeUint64(sealed.EpochNumber, "epoch number")
	if err != nil {
		return nil, err
	}

	inputLower, err := safeUint64(sealed.InputIndexLowerBound, "input lower bound")
	if err != nil {
		return nil, err
	}

	inputUpper, err := safeUint64(sealed.InputIndexUpperBound, "input upper bound")
	if err != nil {
		return nil, err
	}

	deployBlockRaw, err := caller.GetDeploymentBlockNumber(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetDeploymentBlockNumber: %w", err)
	}
	deployBlock, err := safeUint64(deployBlockRaw, "deployment block")
	if err != nil {
		return nil, err
	}

	inputBox, err := caller.GetInputBox(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetInputBox: %w", err)
	}

	factory, err := caller.GetTournamentFactory(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetTournamentFactory: %w", err)
	}

	result := &DaveConsensusResult{
		Type:               "DaveConsensus",
		Address:            formatAddr(addr),
		InputBox:           formatAddr(inputBox),
		Factory:            formatAddr(factory),
		DeploymentBlock:    deployBlock,
		IsFinished:         settleInfo.IsFinished,
		CurrentEpochNumber: epochNumber,
		InputLowerBound:    inputLower,
		InputUpperBound:    inputUpper,
		RootTournament:     formatAddr(sealed.Tournament),
	}
	if result.IsFinished {
		hasWinner := settleInfo.WinnerCommitment != [32]byte{}
		result.HasWinner = &hasWinner
		if hasWinner {
			result.WinnerCommitment = formatHash(settleInfo.WinnerCommitment)
		}
	}
	return result, nil
}
