// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package register

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/require"
)

func TestValidateRegistrationConsensusType(t *testing.T) {
	tests := []struct {
		name               string
		consensusType      model.Consensus
		applicationTypePRT bool
		wantErr            string
	}{
		{
			name:               "prt flag with prt consensus",
			consensusType:      model.Consensus_PRT,
			applicationTypePRT: true,
		},
		{
			name:               "prt flag with authority consensus",
			consensusType:      model.Consensus_Authority,
			applicationTypePRT: true,
			wantErr:            "--prt was passed",
		},
		{
			name:               "prt flag with quorum consensus",
			consensusType:      model.Consensus_Quorum,
			applicationTypePRT: true,
			wantErr:            "--prt was passed",
		},
		{
			name:          "no prt flag with prt consensus",
			consensusType: model.Consensus_PRT,
		},
		{
			name:          "no prt flag with authority consensus",
			consensusType: model.Consensus_Authority,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRegistrationConsensusType(tt.consensusType, tt.applicationTypePRT)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRegisterCommandDoesNotExposeLegacyInputBoxFlags(t *testing.T) {
	require.Nil(t, Cmd.Flags().Lookup("data-availability"))
	require.Nil(t, Cmd.Flags().Lookup("inputbox-from-env"))
	require.Nil(t, Cmd.Flags().Lookup("inputbox-block-number"))
}
