// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//! Complete merkle tree based on Cartesi machine-emulator implementation

use super::{
    get_concat_hash, pristine, proof::Proof, Error,
    LeafSizeGreaterThanRootSizeSnafu, MisalignedAddressSnafu,
    SizeOutOfRangeSnafu, TooManyLeavesSnafu, TreeIsFullSnafu,
    TreeTooLargeSnafu, WordSizeGreaterThanLeafSizeSnafu,
};
use crate::hash::{Digest, Hash, Hasher};

/// Complete merkle tree
///
/// A merkle tree with any number of non-pristine leaves followed by a number of pristine leaves.
/// The tree is optimized to store only the hashes that are not pristine.
#[derive(Debug)]
pub struct Tree {
    log2_root_size: usize,
    log2_leaf_size: usize,
    pristine: pristine::Tree,
    tree: Vec<Level>,
}

impl Tree {
    /// Create a new complete merkle tree
    ///
    /// - `log2_root_size`: Log2 of the size in bytes of the whole merkle tree.
    /// - `log2_leaf_size`: Log2 of the size in bytes of a single leaf.
    /// - `log2_word_size`: Log2 of the size in bytes of a single word. This is used to compute the
    ///                     pristine hash of a leave.
    pub fn new(
        log2_root_size: usize,
        log2_leaf_size: usize,
        log2_word_size: usize,
    ) -> Result<Self, Error> {
        snafu::ensure!(
            log2_leaf_size <= log2_root_size,
            LeafSizeGreaterThanRootSizeSnafu
        );
        snafu::ensure!(
            log2_word_size <= log2_leaf_size,
            WordSizeGreaterThanLeafSizeSnafu
        );
        snafu::ensure!(
            log2_root_size <= std::mem::size_of::<usize>() * 8,
            TreeTooLargeSnafu
        );
        Ok(Self {
            log2_root_size,
            log2_leaf_size,
            pristine: pristine::Tree::new(log2_root_size, log2_word_size)?,
            tree: vec![vec![]; log2_root_size - log2_leaf_size + 1],
        })
    }

    /// Create a new complete merkle tree from non-pristine leaves
    ///
    /// - `leaves`: Array with non-pristine hash leaves bound to the left side of the tree.
    ///
    /// For more information regarding the other parameters, see Tree::new().
    pub fn new_from_leaves(
        log2_root_size: usize,
        log2_leaf_size: usize,
        log2_word_size: usize,
        leaves: Level,
    ) -> Result<Self, Error> {
        let max_len = 1 << (log2_root_size - log2_leaf_size);
        snafu::ensure!(leaves.len() <= max_len, TooManyLeavesSnafu);
        let mut tree =
            Self::new(log2_root_size, log2_leaf_size, log2_word_size)?;
        let level = tree.get_level_mut(log2_leaf_size).expect("cannot fail");
        *level = leaves;
        tree.bubble_up();
        Ok(tree)
    }

    /// Return the tree's root hash
    pub fn get_root_hash(&self) -> &Hash {
        self.get_node_hash(0, self.log2_root_size)
            .expect("cannot fail")
    }

    /// Return proof for a given node
    ///
    /// - `address`: The address is represented by the node index at the level shifted by
    ///              `log2_size`.
    /// - `log2_size`: Log2 of the size in bytes of the subtree.
    pub fn get_proof(
        &self,
        address: usize,
        log2_size: usize,
    ) -> Result<Proof, Error> {
        snafu::ensure!(
            log2_size >= self.log2_leaf_size
                && log2_size <= self.log2_root_size,
            SizeOutOfRangeSnafu
        );
        let aligned_address = (address >> log2_size) << log2_size;
        snafu::ensure!(address == aligned_address, MisalignedAddressSnafu);
        let target_hash = self.get_node_hash(address, log2_size)?.clone();
        let log2_root_size = self.log2_root_size;
        let root_hash = self.get_root_hash().clone();
        let mut proof = Proof::new(
            address,
            log2_size,
            target_hash,
            log2_root_size,
            root_hash,
        )?;
        for log2_sibling_size in log2_size..log2_root_size {
            let sibling_address = address ^ (1 << log2_sibling_size);
            let hash =
                self.get_node_hash(sibling_address, log2_sibling_size)?;
            proof.set_sibling_hash(hash.clone(), log2_sibling_size)?;
        }
        Ok(proof)
    }

    /// Append a new leaf hash to the tree
    ///
    /// - `leaf`: Hash to append.
    pub fn push(&mut self, leaf: Hash) -> Result<(), Error> {
        let max_len = 1 << (self.log2_root_size - self.log2_leaf_size);
        let leaves = self
            .get_level_mut(self.log2_leaf_size)
            .expect("cannot fail");
        snafu::ensure!(leaves.len() < max_len, TreeIsFullSnafu);
        leaves.push(leaf);
        self.bubble_up();
        Ok(())
    }

    /// Return the number of leafs
    pub fn len(&self) -> usize {
        self.get_level(self.log2_leaf_size)
            .expect("cannot fail")
            .len()
    }

    /// Return the hash of a node at a given address
    ///
    /// For more information regarding the other parameters, see Tree::get_proof().
    fn get_node_hash(
        &self,
        address: usize,
        log2_size: usize,
    ) -> Result<&Hash, Error> {
        let address = address >> log2_size;
        let bounds = 1 << (self.log2_root_size - log2_size);
        snafu::ensure!(address < bounds, SizeOutOfRangeSnafu);
        let level = self.get_level(log2_size)?;
        if address < level.len() {
            Ok(&level[address])
        } else {
            self.pristine.get_hash(log2_size)
        }
    }

    /// Update node hashes when a new set of non-pristine nodes is added to the leaf level
    fn bubble_up(&mut self) {
        let mut hasher = Hasher::new();
        // Go bottom up, updating hashes
        for log2_prev_size in self.log2_leaf_size..self.log2_root_size {
            let log2_next_size = log2_prev_size + 1;
            // Extract the next level from self to deal with borrow-checker
            let mut next = vec![];
            std::mem::swap(
                &mut next,
                self.get_level_mut(log2_next_size).expect("cannot fail"),
            );
            let prev = self.get_level(log2_prev_size).expect("cannot fail");
            // Redo last entry (if any) because it may have been constructed
            // from the last non-pristine entry in the previous level paired
            // with a pristine entry (i.e., the previous level was odd).
            let first_entry = if next.is_empty() { 0 } else { next.len() - 1 };
            // Next level needs half as many (rounded up) as previous
            next.resize_with((prev.len() + 1) / 2, Default::default);
            // Last safe entry has two non-pristine leafs
            let last_safe_entry = prev.len() / 2;
            // Do all entries for which we have two non-pristine children
            for i in first_entry..last_safe_entry {
                next[i] = get_concat_hash(
                    &mut hasher,
                    &prev[2 * i],
                    &prev[2 * i + 1],
                );
            }
            // Maybe do last odd entry
            if prev.len() > 2 * last_safe_entry {
                let prev_pristine = self
                    .pristine
                    .get_hash(log2_prev_size)
                    .expect("cannot fail");
                next[last_safe_entry] = get_concat_hash(
                    &mut hasher,
                    &prev[prev.len() - 1],
                    prev_pristine,
                );
            }
            // Put the level back in self
            std::mem::swap(
                &mut next,
                self.get_level_mut(log2_next_size).expect("cannot fail"),
            );
        }
    }

    /// Return the hashes at the given level
    fn get_level(&self, log2_size: usize) -> Result<&Level, Error> {
        let index = self.get_level_index(log2_size)?;
        Ok(&self.tree[index])
    }

    /// Mutable version of Tree::get_level()
    fn get_level_mut(&mut self, log2_size: usize) -> Result<&mut Level, Error> {
        let index = self.get_level_index(log2_size)?;
        Ok(&mut self.tree[index])
    }

    /// Compute the level index given the sub-tree size
    ///
    /// - `log2_size`: Log2 of the size in bytes of the subtree.
    fn get_level_index(&self, log2_size: usize) -> Result<usize, Error> {
        snafu::ensure!(
            log2_size >= self.log2_leaf_size
                && log2_size <= self.log2_root_size,
            SizeOutOfRangeSnafu
        );
        Ok(self.log2_root_size - log2_size)
    }
}

type Level = Vec<Hash>;

#[cfg(test)]
mod tests {
    use super::*;
    use crate::hash::HASH_SIZE;

    fn compare_to_pristine(
        tree: Tree,
        log2_root_size: usize,
        log2_leaf_size: usize,
        log2_word_size: usize,
    ) {
        let pristine =
            pristine::Tree::new(log2_root_size, log2_word_size).unwrap();
        for log2_size in log2_leaf_size..log2_root_size {
            let max_address = 1 << (log2_root_size - log2_size);
            for address in 0..max_address {
                assert_eq!(
                    tree.get_node_hash(address << log2_size, log2_size)
                        .unwrap(),
                    pristine.get_hash(log2_size).unwrap()
                );
            }
        }
    }

    #[test]
    fn test_it_fails_to_create_a_tree_with_leaf_size_greater_than_root_size() {
        let err = Tree::new(2, 3, 0).unwrap_err();
        assert_eq!(err, Error::LeafSizeGreaterThanRootSize);
    }

    #[test]
    fn test_it_fails_to_create_a_tree_with_word_size_greater_than_leaf_size() {
        let err = Tree::new(2, 1, 2).unwrap_err();
        assert_eq!(err, Error::WordSizeGreaterThanLeafSize);
    }

    #[test]
    fn test_it_fails_to_create_that_does_not_fit_in_memory() {
        let err = Tree::new(65, 1, 0).unwrap_err();
        assert_eq!(err, Error::TreeTooLarge);
    }

    #[test]
    fn test_it_is_equals_to_pristine_tree_when_empty() {
        let tree = Tree::new(8, 3, 0).unwrap();
        compare_to_pristine(tree, 8, 3, 0);
    }

    #[test]
    fn test_it_fails_to_create_tree_with_too_many_leaves() {
        // It should have at most 2 leaves
        let leaves = vec![Hash::default(); 3];
        let err = Tree::new_from_leaves(3, 2, 1, leaves).unwrap_err();
        assert_eq!(err, Error::TooManyLeaves);
    }

    #[test]
    fn test_it_is_equals_to_pristine_tree_when_created_with_no_leaves() {
        let leaves = vec![];
        let tree = Tree::new_from_leaves(8, 3, 0, leaves).unwrap();
        compare_to_pristine(tree, 8, 3, 0);
    }

    #[test]
    fn test_it_works_propertly_when_created_with_all_leaves() {
        let leaves = vec![Hash::from([0xFF; HASH_SIZE]); 8];
        let tree = Tree::new_from_leaves(3, 0, 0, leaves).unwrap();
        assert_eq!(tree.get_level(0).unwrap().len(), 8);
        assert_eq!(tree.get_level(1).unwrap().len(), 4);
        assert_eq!(tree.get_level(2).unwrap().len(), 2);
        assert_eq!(tree.get_level(3).unwrap().len(), 1);
        assert_eq!(
            tree.get_root_hash(),
            &Hash::decode("ec06b3285e5018dbd1981c64dbf6e9cea02fa591f0322b51cfbc31a729500928")
        );
    }

    #[test]
    fn test_it_works_propertly_when_created_with_odd_number_of_leaves() {
        let leaves = vec![Hash::from([0xFF; HASH_SIZE]); 3];
        let tree = Tree::new_from_leaves(3, 0, 0, leaves).unwrap();
        assert_eq!(tree.get_level(0).unwrap().len(), 3);
        assert_eq!(tree.get_level(1).unwrap().len(), 2);
        assert_eq!(tree.get_level(2).unwrap().len(), 1);
        assert_eq!(tree.get_level(3).unwrap().len(), 1);
        assert_eq!(
            tree.get_root_hash(),
            &Hash::decode("4d41dd9b105ebb70fbed098caccf3d56b286c690a96202c1357f951cca4dad5d")
        );
    }

    #[test]
    fn test_it_works_properly_when_root_size_equals_to_leaf_size() {
        let leaves = vec![Hash::from([0xFF; HASH_SIZE])];
        let tree = Tree::new_from_leaves(0, 0, 0, leaves).unwrap();
        assert_eq!(tree.get_root_hash(), &Hash::from([0xFF; HASH_SIZE]));
    }

    #[test]
    fn test_it_computes_the_level_index_properly() {
        let tree = Tree::new(3, 1, 0).unwrap();
        assert_eq!(tree.get_level_index(1).unwrap(), 2);
        assert_eq!(tree.get_level_index(2).unwrap(), 1);
        assert_eq!(tree.get_level_index(3).unwrap(), 0);
    }

    #[test]
    fn test_it_fails_to_compute_the_index_when_log2_size_is_out_of_range() {
        let tree = Tree::new(3, 2, 1).unwrap();
        assert_eq!(tree.get_level_index(4).unwrap_err(), Error::SizeOutOfRange);
        assert_eq!(tree.get_level_index(1).unwrap_err(), Error::SizeOutOfRange);
    }

    #[test]
    fn test_it_fails_to_get_proof_when_log2_size_is_out_of_range() {
        let tree = Tree::new(3, 2, 1).unwrap();
        assert_eq!(tree.get_proof(0, 4).unwrap_err(), Error::SizeOutOfRange);
        assert_eq!(tree.get_proof(0, 1).unwrap_err(), Error::SizeOutOfRange);
    }

    #[test]
    fn test_it_fails_to_get_proof_when_address_is_misalign() {
        let tree = Tree::new(3, 2, 1).unwrap();
        assert_eq!(
            tree.get_proof((0 << 2) + 1, 2).unwrap_err(),
            Error::MisalignedAddress
        );
        assert_eq!(
            tree.get_proof((1 << 2) + 1, 2).unwrap_err(),
            Error::MisalignedAddress
        );
    }

    #[test]
    fn test_it_fails_to_get_proof_when_address_is_out_of_bound() {
        let tree = Tree::new(3, 2, 1).unwrap();
        assert_eq!(
            tree.get_proof(2 << 2, 2).unwrap_err(),
            Error::SizeOutOfRange
        );
        assert_eq!(
            tree.get_proof(1 << 3, 2).unwrap_err(),
            Error::SizeOutOfRange
        );
    }

    #[test]
    fn test_it_gets_correct_proof_of_root_node() {
        let tree = Tree::new(3, 2, 1).unwrap();
        let proof = tree.get_proof(0, 3).unwrap();
        let root_hash = tree.pristine.get_hash(3).unwrap();
        assert_eq!(proof.target_address, 0);
        assert_eq!(proof.log2_target_size, 3);
        assert_eq!(&proof.target_hash, root_hash);
        assert_eq!(proof.log2_root_size, 3);
        assert_eq!(&proof.root_hash, root_hash);
        assert_eq!(proof.sibling_hashes, vec![]);
    }

    #[test]
    fn test_it_gets_correct_proof_of_leaf_node_when_empty() {
        let tree = Tree::new(3, 0, 0).unwrap();
        let proof = tree.get_proof(0, 0).unwrap();
        assert_eq!(proof.target_address, 0);
        assert_eq!(proof.log2_target_size, 0);
        assert_eq!(&proof.target_hash, tree.pristine.get_hash(0).unwrap());
        assert_eq!(proof.log2_root_size, 3);
        assert_eq!(&proof.root_hash, tree.pristine.get_hash(3).unwrap());
        assert_eq!(
            proof.sibling_hashes,
            vec![
                tree.pristine.get_hash(0).unwrap().clone(),
                tree.pristine.get_hash(1).unwrap().clone(),
                tree.pristine.get_hash(2).unwrap().clone(),
            ]
        );
    }

    #[test]
    fn test_it_gets_correct_proof_of_leaf_node_when_half_full() {
        let leaves = vec![Hash::from([0xFF; HASH_SIZE]); 3];
        let tree = Tree::new_from_leaves(3, 0, 0, leaves).unwrap();
        let proof = tree.get_proof(3, 0).unwrap();
        assert_eq!(proof.target_address, 3);
        assert_eq!(proof.log2_target_size, 0);
        assert_eq!(&proof.target_hash, tree.pristine.get_hash(0).unwrap());
        assert_eq!(proof.log2_root_size, 3);
        assert_eq!(
            proof.root_hash,
            Hash::decode("4d41dd9b105ebb70fbed098caccf3d56b286c690a96202c1357f951cca4dad5d")
        );
        assert_eq!(
            proof.sibling_hashes,
            vec![
                Hash::from([0xFF; HASH_SIZE]),
                Hash::decode("bd8b151773dbbefd7b0df67f2dcc482901728b6df477f4fb2f192733a005d396"),
                Hash::decode("bb1bfb5bfc9ba6ba8e25341a7b70725d8f74121b9e31dd2314e68e27b8d24244"),
            ]
        );
    }

    #[test]
    fn test_it_pushes_leaf_in_empty_tree() {
        let mut tree = Tree::new(3, 0, 0).unwrap();
        tree.push(Hash::from([0xFF; HASH_SIZE])).unwrap();
        assert_eq!(tree.get_level(0).unwrap().len(), 1);
        assert_eq!(tree.get_level(1).unwrap().len(), 1);
        assert_eq!(tree.get_level(2).unwrap().len(), 1);
        assert_eq!(tree.get_level(3).unwrap().len(), 1);
        assert_eq!(
            tree.get_root_hash(),
            &Hash::decode("da0df8c5459cfe821402ad885e95e31e6bb4abf940e13b4c69afa5245b4eeb7d")
        );
    }

    #[test]
    fn test_it_pushes_leaf_in_almost_full_tree() {
        let mut tree = Tree::new(3, 0, 0).unwrap();
        for i in 0..8 {
            tree.push(Hash::from([0xFF; HASH_SIZE])).unwrap();
            assert_eq!(tree.get_level(0).unwrap().len(), i + 1);
        }
        assert_eq!(tree.get_level(1).unwrap().len(), 4);
        assert_eq!(tree.get_level(2).unwrap().len(), 2);
        assert_eq!(tree.get_level(3).unwrap().len(), 1);
        assert_eq!(
            tree.get_root_hash(),
            &Hash::decode("ec06b3285e5018dbd1981c64dbf6e9cea02fa591f0322b51cfbc31a729500928")
        );
    }

    #[test]
    fn test_it_fails_to_push_leaf_in_full_tree() {
        let leaves = vec![Hash::from([0xFF; HASH_SIZE]); 8];
        let mut tree = Tree::new_from_leaves(3, 0, 0, leaves).unwrap();
        let err = tree.push(Hash::default()).unwrap_err();
        assert_eq!(err, Error::TreeIsFull);
    }
}
