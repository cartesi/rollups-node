// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use eth_state_fold::Foldable;
use eth_state_fold_types::ethers::prelude::{ContractError, Middleware};
use std::error::Error;
use std::fmt::{Display, Formatter};

#[derive(Debug)]
pub struct FoldableError(Box<dyn Error>);

impl Display for FoldableError {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        self.0.fmt(f)
    }
}

impl std::error::Error for FoldableError {}

impl From<Box<dyn Error>> for FoldableError {
    fn from(error: Box<dyn Error>) -> Self {
        Self(error)
    }
}

impl<M: Middleware + 'static> From<ContractError<M>> for FoldableError {
    fn from(contract_error: ContractError<M>) -> Self {
        FoldableError(contract_error.into())
    }
}

impl<M: Middleware + 'static, F: Foldable + 'static>
    From<eth_state_fold::error::FoldableError<M, F>> for FoldableError
where
    <F as Foldable>::Error: Send + Sync,
{
    fn from(
        contract_error: eth_state_fold::error::FoldableError<M, F>,
    ) -> Self {
        FoldableError(contract_error.into())
    }
}
