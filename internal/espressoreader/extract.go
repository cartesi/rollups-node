package espressoreader

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

type SigAndData struct {
	TypedData apitypes.TypedData `json:"typedData"`
	Account   string             `json:"account"`
	Signature string             `json:"signature"`
}

func ExtractSigAndData(raw string) (common.Address, apitypes.TypedData, string, error) {
	var sigAndData SigAndData
	decodedRaw, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return common.HexToAddress("0x"), apitypes.TypedData{}, "", fmt.Errorf("decode base64: %w", err)
	}

	if err := json.Unmarshal(decodedRaw, &sigAndData); err != nil {
		return common.HexToAddress("0x"), apitypes.TypedData{}, "", fmt.Errorf("unmarshal sigAndData: %w", err)
	}

	signature, err := hexutil.Decode(sigAndData.Signature)
	if err != nil {
		return common.HexToAddress("0x"), apitypes.TypedData{}, "", fmt.Errorf("decode signature: %w", err)
	}
	sigHash := crypto.Keccak256Hash(signature).String()

	typedData := sigAndData.TypedData
	dataHash, _, err := apitypes.TypedDataAndHash(typedData)
	if err != nil {
		return common.HexToAddress("0x"), apitypes.TypedData{}, "", fmt.Errorf("typed data hash: %w", err)
	}

	// update the recovery id
	// https://github.com/ethereum/go-ethereum/blob/55599ee95d4151a2502465e0afc7c47bd1acba77/internal/ethapi/api.go#L442
	signature[64] -= 27

	// get the pubkey used to sign this signature
	sigPubkey, err := crypto.Ecrecover(dataHash, signature)
	if err != nil {
		return common.HexToAddress("0x"), apitypes.TypedData{}, "", fmt.Errorf("ecrecover: %w", err)
	}
	pubkey, err := crypto.UnmarshalPubkey(sigPubkey)
	if err != nil {
		return common.HexToAddress("0x"), apitypes.TypedData{}, "", fmt.Errorf("unmarshal: %w", err)
	}
	address := crypto.PubkeyToAddress(*pubkey)

	return address, typedData, sigHash, nil
}
