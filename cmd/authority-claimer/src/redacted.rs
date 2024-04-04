// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use std::fmt;
pub use url::{self, Url};

/// Wrapper that redacts the entire field
#[derive(Clone)]
pub struct Redacted<T: Clone>(T);

impl<T: Clone> Redacted<T> {
    pub fn new(data: T) -> Redacted<T> {
        Self(data)
    }

    pub fn inner(&self) -> &T {
        &self.0
    }

    pub fn into_inner(self) -> T {
        self.0
    }
}

impl<T: Clone> fmt::Debug for Redacted<T> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "[REDACTED]")
    }
}

#[test]
fn redacts_debug_fmt() {
    let password = Redacted::new("super-security");
    assert_eq!(format!("{:?}", password), "[REDACTED]");
}

/// Wrapper that redacts the credentials in an URL
#[derive(Clone)]
pub struct RedactedUrl(Url);

impl RedactedUrl {
    pub fn new(url: Url) -> Self {
        Self(url)
    }

    pub fn inner(&self) -> &Url {
        &self.0
    }

    pub fn into_inner(self) -> Url {
        self.0
    }
}

impl fmt::Debug for RedactedUrl {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let mut url = self.inner().clone();
        let result = {
            if url.cannot_be_a_base() {
                Err(())
            } else {
                Ok(())
            }
        }
        .and_then(|_| {
            if !url.username().is_empty() {
                url.set_username("***")
            } else {
                Ok(())
            }
        })
        .and_then(|_| {
            if url.password().is_some() {
                url.set_password(Some("***"))
            } else {
                Ok(())
            }
        });
        match result {
            Ok(_) => write!(f, "{}", url.as_str()),
            Err(_) => write!(f, "[NON-BASE URL REDACTED]"),
        }
    }
}

#[test]
fn redacts_valid_url_without_credentials() {
    let url = RedactedUrl::new(Url::parse("http://example.com/").unwrap());
    assert_eq!(format!("{:?}", url), "http://example.com/");
}

#[test]
fn redacts_valid_url_with_username() {
    let url =
        RedactedUrl::new(Url::parse("http://james@example.com/").unwrap());
    assert_eq!(format!("{:?}", url), "http://***@example.com/");
}

#[test]
fn redacts_valid_url_with_password() {
    let url =
        RedactedUrl::new(Url::parse("http://:bond@example.com/").unwrap());
    assert_eq!(format!("{:?}", url), "http://:***@example.com/");
}

#[test]
fn redacts_valid_url_with_full_credentials() {
    let url =
        RedactedUrl::new(Url::parse("http://james:bond@example.com/").unwrap());
    assert_eq!(format!("{:?}", url), "http://***:***@example.com/");
}

#[test]
fn redacts_non_base_url() {
    let url = RedactedUrl::new(Url::parse("james:bond@example.com").unwrap());
    assert_eq!(format!("{:?}", url), "[NON-BASE URL REDACTED]");
}
