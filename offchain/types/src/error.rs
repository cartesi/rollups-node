// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use anyhow::Error;
use state_fold::state_fold::Foldable;
use state_fold_types::ethers::prelude::{ContractError, Middleware};
use std::fmt::{Display, Formatter};

#[derive(Debug)]
pub struct FoldableError(Error);

impl Display for FoldableError {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        self.0.fmt(f)
    }
}

impl std::error::Error for FoldableError {}

impl From<Error> for FoldableError {
    fn from(error: Error) -> Self {
        Self(error)
    }
}

impl<M: Middleware + 'static> From<ContractError<M>> for FoldableError {
    fn from(contract_error: ContractError<M>) -> Self {
        FoldableError(contract_error.into())
    }
}

impl<M: Middleware + 'static, F: Foldable + 'static>
    From<state_fold::state_fold::error::FoldableError<M, F>> for FoldableError
where
    <F as Foldable>::Error: Send + Sync,
{
    fn from(
        contract_error: state_fold::state_fold::error::FoldableError<M, F>,
    ) -> Self {
        FoldableError(contract_error.into())
    }
}
