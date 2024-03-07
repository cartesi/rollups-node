// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"fmt"
	"os"
)

// ------------------------------------------------------------------------------------------------
// Custom GETs
// ------------------------------------------------------------------------------------------------

func getAuth() (*Auth, error) {
	// getting the (optional) account index
	index := getCartesiAuthMnemonicAccountIndex()

	// if the mnemonic is coming from an environment variable
	if mnemonic := getCartesiAuthMnemonic(); mnemonic != nil && index != nil {
		var auth Auth = AuthMnemonic{Mnemonic: *mnemonic, AccountIndex: *index}
		return &auth, nil
	}

	// if the mnemonic is coming from a file
	if file := getCartesiAuthMnemonicFile(); file != nil {
		mnemonic, err := os.ReadFile(*file)
		if err != nil {
			return nil, fmt.Errorf("mnemonic file error: %s", err)
		}
		var auth Auth = AuthMnemonic{Mnemonic: string(mnemonic), AccountIndex: *index}
		return &auth, nil
	}

	// if we are not using mnemonics, but AWS authentication
	keyID := getCartesiAuthAwsKmsKeyId()
	region := getCartesiAuthAwsKmsRegion()
	if keyID == nil || region == nil {
		return nil, fmt.Errorf("missing auth environment variables")
	}
	var auth Auth = AuthAWS{KeyID: *keyID, Region: *region}
	return &auth, nil
}
