// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use anyhow::{anyhow, Result};
use ethers::contract::Abigen;
use proc_macro2::token_stream::IntoIter;
use proc_macro2::{Delimiter, Group, TokenStream, TokenTree};
use quote::quote;
use serde_json::Value;
use std::error;
use std::io::{Read, Write};
use std::process::{Command, Stdio};

pub use {crate::contract_include as include, crate::contract_path as path};

/// Generates file path for the contract's bindings.
///
/// # Examples
/// The example below illustrates how to create a file during compile time (i.e.
/// in a build script).
///
/// ```no_run
/// # use std::fs::File;
/// # use state_fold_types::contract;
/// # fn main() -> Result<(), Box<dyn std::error::Error>> {
/// let mut output = File::create(contract::path!("rollups_contract.rs"))?;
/// # Ok(())
/// # }
/// ```
#[macro_export]
macro_rules! contract_path {
    ($path:expr) => {
        format!("{}/{}", std::env::var("OUT_DIR").unwrap(), $path)
    };
}

/// Includes contract's bindings.
///
/// # Examples
/// The example below includes a contract called `rollups_contract`. It has the
/// same effect as importing `rollups_contract::*`.
///
/// ```ignore
/// # use state_fold_types::contract;
/// contract::include!("rollups_contract");
/// ```
#[macro_export]
macro_rules! contract_include {
    ($path:expr) => {
        include!(concat!(env!("OUT_DIR"), "/", $path, ".rs"));
    };
}

/// Generates type-safe contract bindings from a contract's ABI. Uses [`Abigen`]
/// under the hood.
///
/// # Examples
/// Usually you would put code similar to this in your `build.rs`.
///
/// ```no_run
/// # use std::fs::File;
/// # use state_fold_types::contract;
/// # fn main() -> Result<(), Box<dyn std::error::Error>> {
/// let source = File::open("../../artifacts/contracts/RollupsImpl.sol/RollupsImpl.abi")?;
/// let mut output = File::create(contract::path!("rollups_contract.rs"))?;
///
/// contract::write("RollupsImpl", source, output)?;
/// # Ok(())
/// # }
/// ```
///
/// [`Abigen`]: ethers::contract::Abigen
pub fn write<R, W>(
    contract_name: &str,
    source: R,
    mut output: W,
) -> Result<(), Box<dyn error::Error>>
where
    R: Read,
    W: Write,
{
    let source: Value = serde_json::from_reader(source)?;
    let abi_source = serde_json::to_string(&source)?;

    let bindings = Abigen::new(contract_name, abi_source)?.generate()?;
    let tokens = bindings.into_tokens();

    let tokens = self::replace_ethers_crates(tokens);
    let raw = tokens.to_string();
    let formatted = self::format(&raw).unwrap_or(raw);

    output.write_all(formatted.as_bytes())?;

    Ok(())
}

/// Formats the raw input source string and return formatted output using
/// locally installed `rustfmt`.
fn format<S>(source: S) -> Result<String>
where
    S: AsRef<str>,
{
    let mut rustfmt = Command::new("rustfmt")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .spawn()?;

    {
        let stdin = rustfmt.stdin.as_mut().ok_or_else(|| {
            anyhow!("stdin was not created for `rustfmt` child process")
        })?;
        stdin.write_all(source.as_ref().as_bytes())?;
    }

    let output = rustfmt.wait_with_output()?;
    if !output.status.success() {
        return Err(anyhow!(
            "`rustfmt` exited with code {}:\n{}",
            output.status,
            String::from_utf8_lossy(&output.stderr),
        ));
    }

    let stdout = String::from_utf8(output.stdout)?;
    Ok(stdout)
}

/// Changes ethers crates to the state_fold_types reexport.
///
/// This way, updating the ethers version is trivial, and we make sure every
/// contract bindings use it.
fn replace_ethers_crates(stream: TokenStream) -> TokenStream {
    look_for_module(TokenStream::new(), &mut stream.into_iter())
}

/// Find the first `mod` statement and then
/// [`look_for_group`][self::look_for_group]
fn look_for_module(
    mut new_stream: TokenStream,
    stream: &mut IntoIter,
) -> TokenStream {
    match stream.next() {
        None => new_stream,
        Some(next) => {
            let found =
                matches!(&next, TokenTree::Ident(ident) if ident == "mod");

            new_stream.extend([next]);

            if found {
                look_for_group(new_stream, stream)
            } else {
                look_for_module(new_stream, stream)
            }
        }
    }
}

/// Find opening braces `{` and then [`look_for_use`][self::look_for_use] within
/// the tokens inside. Then goes to [`look_for_module`][self::look_for_module]
/// on the subsequent tokens after closing brace `}`.
fn look_for_group(
    mut new_stream: TokenStream,
    stream: &mut IntoIter,
) -> TokenStream {
    match stream.next() {
        None => new_stream,
        Some(next) => {
            let found = matches!(&next, TokenTree::Group(group) if matches!(group.delimiter(), Delimiter::Brace));

            if found {
                if let TokenTree::Group(group) = &next {
                    let group_stream = look_for_use(
                        TokenStream::new(),
                        &mut group.stream().into_iter(),
                    );

                    new_stream.extend([TokenTree::Group(Group::new(
                        group.delimiter(),
                        group_stream,
                    ))]);
                }

                look_for_module(new_stream, stream)
            } else {
                new_stream.extend([next]);

                look_for_group(new_stream, stream)
            }
        }
    }
}

/// Find the first `use` statement and preface it with this:
/// ```
/// mod ethers_core {
///     pub use state_fold_types::ethers::core::*;
/// }
/// mod ethers_providers {
///     pub use state_fold_types::ethers::providers::*;
/// }
/// mod ethers_contract {
///     pub use state_fold_types::ethers::contract::*;
/// }
/// mod ethers {
///     pub use state_fold_types::ethers::*;
/// }
/// ```
/// Then goes to [`look_for_module`][self::look_for_module] on the subsequent
/// tokens.
fn look_for_use(
    mut new_stream: TokenStream,
    stream: &mut IntoIter,
) -> TokenStream {
    match stream.next() {
        None => new_stream,
        Some(next) => {
            let found =
                matches!(&next, TokenTree::Ident(ident) if ident == "use");

            if found {
                new_stream.extend([quote!(
                    mod ethers_core {
                        pub use state_fold_types::ethers::core::*;
                    }
                    mod ethers_providers {
                        pub use state_fold_types::ethers::providers::*;
                    }
                    mod ethers_contract {
                        pub use state_fold_types::ethers::contract::*;
                    }
                    mod ethers {
                        pub use state_fold_types::ethers::*;
                    }
                )]);
                new_stream.extend([next]);

                look_for_module(new_stream, stream)
            } else {
                new_stream.extend([next]);

                look_for_use(new_stream, stream)
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use quote::quote;

    #[test]
    fn test_replacing_ethers_crates_uses_correct_crates() {
        let input = quote! {
            mod prdel {
                use ethers::whatever;
            }
        };
        let expected_output = quote! {
            mod prdel {
                mod ethers_core {
                    pub use state_fold_types::ethers::core::*;
                }
                mod ethers_providers {
                    pub use state_fold_types::ethers::providers::*;
                }
                mod ethers_contract {
                    pub use state_fold_types::ethers::contract::*;
                }
                mod ethers {
                    pub use state_fold_types::ethers::*;
                }
                use ethers::whatever;
            }
        }
        .to_string();

        let actual_output = replace_ethers_crates(input).to_string();

        assert_eq!(expected_output, actual_output);
    }
}
