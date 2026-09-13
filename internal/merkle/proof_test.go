// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
)

type merkleProof struct {
	Index    uint64        `json:"index"`
	Hash     common.Hash   `json:"hash"`
	RootHash common.Hash   `json:"RootHash"`
	Siblings []common.Hash `json:"Siblings"`
}

type merkleHashEntry struct {
	Hash     common.Hash   `json:"hash"`
	RootHash common.Hash   `json:"RootHash"`
	Siblings []common.Hash `json:"Siblings"`
}

type merkleDataPoint struct {
	Proof  merkleProof       `json:"proof"`
	Hashes []merkleHashEntry `json:"hashes"`
}

func load(t *testing.T, fileName string) merkleDataPoint {
	var mdp merkleDataPoint

	contents, err := os.ReadFile(fileName)
	assert.Nil(t, err)

	assert.Nil(t, json.Unmarshal(contents, &mdp))
	return mdp
}

// Template function to create new datasets
//func TestCreateProof(t *testing.T) {
//	leaves := []common.Hash{
//		common.HexToHash("0x00"),
//		common.HexToHash("0x00"),
//	}
//	root, siblings, _ := CreateProofs(leaves, TREE_DEPTH)
//
//	mdp := merkleDataPoint{
//		Proof: merkleProof{
//			Index:    uint64(len(leaves)-1),
//			Hash:     leaves[0],
//			RootHash: root,
//			Siblings: siblings[len(siblings)-TREE_DEPTH:], // take last
//		},
//		Hashes: []merkleHashEntry{
//			{
//				Hash: leaves[0],
//				RootHash: root,
//				Siblings: siblings[:TREE_DEPTH],
//			}, {
//				Hash: leaves[0],
//				RootHash: root,
//				Siblings: siblings[TREE_DEPTH:2*TREE_DEPTH],
//			},
//		},
//	}
//	s, err := json.MarshalIndent(mdp, "", "\t")
//	assert.Nil(t, err)
//	fmt.Println(string(s))
//}

func TestRootHash(t *testing.T) {
	leaves := []common.Hash{
		common.HexToHash("0x00"),
	}
	root, siblings, _ := CreateProofs(leaves, TREE_DEPTH)

	postContext := CreatePostContext()
	assert.Equal(t, root, postContext[TREE_DEPTH])

	siblings2, _ := ComputeSiblingsMatrix(postContext, leaves, postContext, 0)
	assert.Equal(t, siblings, siblings2)
}

func TestIncorrectCreateProofsLevel(t *testing.T) {
	leaves := []common.Hash{
		common.HexToHash("0x00"),
		common.HexToHash("0x00"),
	}
	_, _, err := CreateProofs(leaves, 0)
	assert.NotNil(t, err)
}

func TestComputSiblingsMatrixAssertions(t *testing.T) {
	var err error
	outputs := []common.Hash{{}}
	post := CreatePostContext() // always the same
	pre := post
	index := uint64(0)

	_, err = ComputeSiblingsMatrix(pre, []common.Hash{}, post, index)
	assert.NotNil(t, err)

	_, err = ComputeSiblingsMatrix(nil, outputs, post, index)
	assert.NotNil(t, err)

	_, err = ComputeSiblingsMatrix(pre, outputs, nil, index)
	assert.NotNil(t, err)
}

// check if creating a pre context from proof and then computing the siblings
// with ComputeSiblingsMatrix matches the reference values encoded in json.
func TestComputeSiblingsMatrix(t *testing.T) {
	post := CreatePostContext() // always the same
	dataSet := []string{
		"dataset/000_add-1_tree-0.json", // insert 1 element on a merkle with 0 elements
		"dataset/001_add-1_tree-1.json", // insert 1 element on a merkle with 1 element
		"dataset/002_add-2_tree-1.json", // insert 2 element on a merkle with 1 element
		"dataset/003_add-2_tree-2.json", // insert 2 element on a merkle with 2 element
	}
	for _, testName := range dataSet {
		t.Run(testName, func(t *testing.T) {
			dataPoint := load(t, testName)
			proof := &dataPoint.Proof
			hashes := dataPoint.Hashes

			// pristine if there is no previous state
			pre := post
			index := uint64(0)

			// compute `pre` if the merkle has state
			if len(proof.Siblings) != 0 {
				// sanity check. Proof must comput rootHash
				assert.Equal(t, proof.RootHash,
					ComputeRootHashFromProof(proof.Index, proof.Hash, proof.Siblings))
				pre = CreatePreContextFromProof(proof.Index, proof.Hash, proof.Siblings)
				index = proof.Index + 1
			}

			// gather hashes
			outputs := []common.Hash{}
			for _, entry := range hashes {
				outputs = append(outputs, entry.Hash)
			}

			// compute siblings. must match reference.
			siblings, err := ComputeSiblingsMatrix(pre, outputs, post, index)
			assert.Nil(t, err)
			for i := range outputs {
				begin := TREE_DEPTH * i
				end := TREE_DEPTH * (i + 1)
				assert.Equal(t, hashes[i].Siblings, siblings[begin:end])
			}
		})
	}
}
