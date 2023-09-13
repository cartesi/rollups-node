// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use rusoto_core::credential::{
    AwsCredentials, CredentialsError, DefaultCredentialsProvider,
    ProvideAwsCredentials,
};
use rusoto_sts::WebIdentityProvider;
use std::env;

pub const WEB_IDENTITY_ENV_VARS: [&str; 3] = [
    "AWS_WEB_IDENTITY_TOKEN_FILE",
    "AWS_ROLE_ARN",
    "AWS_ROLE_SESSION_NAME",
];

/// The `AwsCredentialsProvider` wraps around a `ProvideAwsCredentials`
/// trait object, and reimplements the `ProvideAwsCredentials` trait.
///
/// The underlying implementation can be either a `rusoto_sts::WebIdentityProvider`
/// or a `rusoto_core::credential::DefaultCredentialsProvider`. It prioritizes
/// instantiating a `WebIdentityProvider` if the correct environment variables
/// are set.
pub struct AwsCredentialsProvider(Box<dyn ProvideAwsCredentials + Send + Sync>);

#[async_trait]
impl ProvideAwsCredentials for AwsCredentialsProvider {
    async fn credentials(&self) -> Result<AwsCredentials, CredentialsError> {
        self.0.credentials().await
    }
}

impl AwsCredentialsProvider {
    pub fn new() -> Result<Self, CredentialsError> {
        for env_var in WEB_IDENTITY_ENV_VARS {
            if env::var(env_var).is_err() {
                tracing::trace!("ENV VAR {} is not set", env_var);
                tracing::trace!("instantiating default provider");
                return DefaultCredentialsProvider::new()
                    .map(Box::new)
                    .map(|inner| Self(inner));
            }
        }
        tracing::trace!("instantiating web identity provider");
        Ok(Self(Box::new(WebIdentityProvider::from_k8s_env())))
    }
}

#[cfg(test)]
pub mod tests {
    use serial_test::serial;
    use std::env;
    use tracing_test::traced_test;

    use crate::signer::aws_credentials::{
        AwsCredentialsProvider, WEB_IDENTITY_ENV_VARS,
    };

    // --------------------------------------------------------------------------------------------
    // new
    //   These and any other tests that use credential vars are #[serial]
    //   because there might be ENV VAR concurrency issues if they run
    //   in parallel.
    // --------------------------------------------------------------------------------------------

    #[test]
    #[serial]
    #[traced_test]
    fn new_default_provider_when_one_web_identity_var_is_missing() {
        for i in 0..3 {
            clean_web_identity_vars();
            set_web_identity_vars();
            remove_web_identity_var(i);
            let result = AwsCredentialsProvider::new();
            assert!(result.is_ok());
            assert!(!logs_contain("instantiating web identity provider"));
            assert!(logs_contain("instantiating default provider"));
        }
    }

    #[test]
    #[serial]
    #[traced_test]
    fn new_default_provider_when_two_web_identity_vars_are_missing() {
        for i in 0..3 {
            clean_web_identity_vars();
            set_web_identity_var(i);
            let result = AwsCredentialsProvider::new();
            assert!(result.is_ok());
            assert!(!logs_contain("instantiating web identity provider"));
            assert!(logs_contain("instantiating default provider"));
        }
    }

    #[test]
    #[serial]
    #[traced_test]
    fn new_default_provider_when_all_web_identity_vars_are_missing() {
        clean_web_identity_vars();
        let result = AwsCredentialsProvider::new();
        assert!(result.is_ok());
        assert!(!logs_contain("instantiating web identity provider"));
        assert!(logs_contain("instantiating default provider"));
    }

    #[test]
    #[serial]
    #[traced_test]
    fn new_web_identity_provider_when_no_web_identity_vars_are_missing() {
        clean_web_identity_vars();
        set_web_identity_vars();
        let result = AwsCredentialsProvider::new();
        assert!(result.is_ok());
        assert!(!logs_contain("instantiating default provider"));
        assert!(logs_contain("instantiating web identity provider"));
    }

    // --------------------------------------------------------------------------------------------
    // env vars
    //   Used by other tests.
    // --------------------------------------------------------------------------------------------

    fn set_web_identity_var(i: usize) {
        env::set_var(WEB_IDENTITY_ENV_VARS[i], "irrelevant");
    }

    fn set_web_identity_vars() {
        for env_var in WEB_IDENTITY_ENV_VARS {
            env::set_var(env_var, "irrelevant");
        }
    }

    fn clean_web_identity_vars() {
        for env_var in WEB_IDENTITY_ENV_VARS {
            env::remove_var(env_var);
        }
    }

    fn remove_web_identity_var(i: usize) {
        env::remove_var(WEB_IDENTITY_ENV_VARS[i]);
    }
}
