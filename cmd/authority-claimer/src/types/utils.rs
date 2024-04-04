// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use std::cmp::Ordering;
use std::iter::Peekable;

pub struct MergeAscending<L, R>
where
    L: Iterator<Item = R::Item>,
    R: Iterator,
{
    left: Peekable<L>,
    right: Peekable<R>,
}

impl<L, R> Iterator for MergeAscending<L, R>
where
    L: Iterator<Item = R::Item>,
    R: Iterator,
    L::Item: Ord,
{
    type Item = L::Item;

    fn next(&mut self) -> Option<L::Item> {
        let which = match (self.left.peek(), self.right.peek()) {
            (Some(l), Some(r)) => Some(l.cmp(r)),
            (Some(_), None) => Some(Ordering::Less),
            (None, Some(_)) => Some(Ordering::Greater),
            (None, None) => None,
        };

        match which {
            Some(Ordering::Less) => self.left.next(),
            Some(Ordering::Equal) => self.left.next(),
            Some(Ordering::Greater) => self.right.next(),
            None => None,
        }
    }

    fn size_hint(&self) -> (usize, Option<usize>) {
        let (ll, lu) = self.left.size_hint();
        let (rl, ru) = self.right.size_hint();

        let l = ll + rl;
        let u = match (lu, ru) {
            (Some(l), Some(r)) => Some(l + r),
            _ => None,
        };

        (l, u)
    }
}
