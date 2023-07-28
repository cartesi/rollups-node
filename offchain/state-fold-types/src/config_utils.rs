// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use serde::de::DeserializeOwned;
use snafu::{ResultExt, Snafu};
use std::fs;

#[derive(Debug, Snafu)]
pub enum Error {
    #[snafu(display(
        "Error while loading configuration file {}, error: {}",
        file_path,
        source
    ))]
    FileError {
        file_path: String,
        source: std::io::Error,
    },

    #[snafu(display(
        "Error while parsing configuration file {}, error: {}",
        file_path,
        source
    ))]
    FileParseError {
        file_path: String,
        source: toml::de::Error,
    },
}

pub type Result<T> = std::result::Result<T, Error>;

pub fn load_config_file<T: Default + DeserializeOwned>(
    config_file: Option<String>,
) -> Result<T> {
    match config_file {
        Some(config_path) => {
            let s = fs::read_to_string(&config_path).context(FileSnafu {
                file_path: config_path.clone(),
            })?;

            let file_config: T =
                toml::from_str(&s).context(FileParseSnafu {
                    file_path: config_path.clone(),
                })?;

            Ok(file_config)
        }
        None => Ok(T::default()),
    }
}
