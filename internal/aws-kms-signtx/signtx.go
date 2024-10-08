/* Package signtx provides utilities for dealing with private keys stored
 * remotely in a Amazon KMS secure server.
 *
 * Without local access to the private key, signing a message involves creating
 * a digest for it, sending it to KMS server, signing it there, then retrieving
 * the signature. */
package signtx

import (
	"context"
	"crypto/ecdsa"
	"encoding/asn1"
	"errors"
	"math/big"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

/* This is the signature for a function that takes a transaction, passes it
 * through a signer to obtain a message digest, creates a signature and embeds
 * it into the transaction itself. */
type SignTxFn = func(tx *types.Transaction, s types.Signer) (*types.Transaction, error)

/* AWS sometimes reply with a `r` larger than 32bytes padded on the left with
 * zeros. Trim it down to a total of 32bytes */
func normalizeR(R []byte) ([]byte, error) {
	if len(R) <= 32 {
		return R, nil
	}
	for i := 0; i < len(R)-32; i++ {
		if R[i] != 0 { // must be padding
			return nil, errors.New("malformed `r` component")
		}
	}
	return R[len(R)-32:], nil
}

/* normalize `s` to the lower half of N according to EIP-2
 * ref. https://eips.ethereum.org/EIPS/eip-2 */
func normalizeS(S []byte) []byte {
	N := crypto.S256().Params().N
	halfN := new(big.Int).Div(N, big.NewInt(2)) //nolint:mnd
	SBI := new(big.Int).SetBytes(S)

	if SBI.Cmp(halfN) > 0 {
		S = new(big.Int).Sub(N, SBI).Bytes()
	}
	return S
}

/* Compute the final component `v` of the ethereum signature, one KMS doesn't
 * provide, and assemble `r`, `s`, and `v` into the signature byte array.
 * Format is: [ R || S || V ], check signature_cgo.go for details.
 *
 * This `v` field is an ethereum extension to the ECDSA `r` and `s` signature
 * values used to facilitate the retrieval of the public key from the hash +
 * signature pair. We use this property to compute `v` by trial and error. One
 * of the values of `v` will hold ecrecover(hash, sig) == publicKey, and that
 * is the one ethereum wants. */
func assembleSignature(r []byte, s []byte, hash []byte, key []byte) ([]byte, error) {
	sig := make([]byte, 65)

	// align `s` and `r` in case they have less then 32bytes in size
	copy(sig[32-len(r):], r)
	copy(sig[64-len(s):], s)

	for i := byte(0); i < 2; i++ {
		sig[64] = i
		pub, err := crypto.Ecrecover(hash, sig[:])
		if err != nil {
			return nil, err
		}
		if reflect.DeepEqual(pub, key) {
			return sig, nil
		}
	}
	return sig, errors.New("failed to compute signature")
}

/* Create a SignTxFn that uses the KMS infrastructure from AWS for signing.
 * The function wraps a context ctx, KMS client and arn to access and identify
 * the signing key. */
func CreateAWSSignTxFn(
	ctx context.Context,
	client *kms.Client,
	arn *string,
) (SignTxFn, *ecdsa.PublicKey, common.Address, error) {
	publicKeyBytes, err := GetPublicKeyBytes(ctx, client, arn)
	if err != nil {
		return nil, nil, common.Address{}, err
	}
	publicKey, err := crypto.UnmarshalPubkey(publicKeyBytes)
	if err != nil {
		return nil, nil, common.Address{}, err
	}
	return func(tx *types.Transaction, signer types.Signer) (*types.Transaction, error) {
		hash := signer.Hash(tx).Bytes()
		signOutput, err := client.Sign(ctx, &kms.SignInput{
			KeyId:            arn,
			Message:          hash,
			SigningAlgorithm: "ECDSA_SHA_256",
			MessageType:      "DIGEST",
		})
		if err != nil {
			return nil, err
		}

		/* AWS returns the signature wrapped in a DER-encoded object.
		 * Try to unwrap it before continuing. */
		type ecdsaSigValue struct {
			R asn1.RawValue
			S asn1.RawValue
		}
		var asn1sig ecdsaSigValue
		_, err = asn1.Unmarshal(signOutput.Signature, &asn1sig)
		if err != nil {
			return nil, err
		}

		S := normalizeS(asn1sig.S.Bytes)
		R, err := normalizeR(asn1sig.R.Bytes)
		if err != nil {
			return nil, err
		}
		signature, err := assembleSignature(R, S, hash, publicKeyBytes)
		if err != nil {
			return nil, err
		}
		return tx.WithSignature(signer, signature[:])
	}, publicKey, crypto.PubkeyToAddress(*publicKey), nil
}

func GetPublicKeyBytes(ctx context.Context, client *kms.Client, Arn *string) ([]byte, error) {
	publicKeyOutput, err := client.GetPublicKey(ctx, &kms.GetPublicKeyInput{
		KeyId: Arn,
	})
	if err != nil {
		return nil, err
	}

	/* AWS returns the public key wrapped in a DER-encoded object. Unwrap
	 * it before returning
	 * ref:
	 *   https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/kms#GetPublicKeyOutput
	 *   https://datatracker.ietf.org/doc/html/rfc5280 (p.16-17) */
	type algorithmIdentifier struct {
		Algorithm  asn1.ObjectIdentifier
		Parameters asn1.ObjectIdentifier // Optional by the spec
	}
	type subjectPublicKeyInfo struct {
		Algorithm        algorithmIdentifier
		SubjectPublicKey asn1.BitString
	}
	var asn1key subjectPublicKeyInfo
	_, err = asn1.Unmarshal(publicKeyOutput.PublicKey, &asn1key)
	if err != nil {
		return nil, err
	}
	return asn1key.SubjectPublicKey.Bytes, nil
}

/* Wrap a KMS SignTx into a geth TransactOpts.
 *
 * similar to NewKeyedTransactorWithChainID */
func CreateAWSTransactOpts(
	ctx context.Context,
	client *kms.Client,
	arn *string,
	signer types.Signer,
) (*bind.TransactOpts, error) {
	SignTxFn, _, keyAddress, err := CreateAWSSignTxFn(ctx, client, arn)
	if err != nil {
		return nil, err
	}
	return &bind.TransactOpts{
		From:    keyAddress,
		Signer:  func(
			address common.Address,
			tx *types.Transaction,
		) (*types.Transaction, error) {
			if address != keyAddress {
				return nil, bind.ErrNotAuthorized
			}
			return SignTxFn(tx, signer)
		},
		Context: ctx,
	}, nil
}
