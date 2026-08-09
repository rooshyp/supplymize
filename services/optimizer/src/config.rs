use std::{net::SocketAddr, time::Duration};

use thiserror::Error;

const DEFAULT_BIND_ADDR: &str = "0.0.0.0:8082";
const DEFAULT_BODY_LIMIT_BYTES: usize = 2 * 1024 * 1024;
const DEFAULT_REQUEST_TIMEOUT_SECONDS: u64 = 15;

#[derive(Clone, Debug)]
pub struct Config {
    pub bind_addr: SocketAddr,
    pub body_limit_bytes: usize,
    pub request_timeout: Duration,
}

impl Config {
    /// Loads runtime configuration from environment variables.
    ///
    /// - `SUPPLYMIZE_OPTIMIZER_ADDR` (default `0.0.0.0:8082`)
    /// - `SUPPLYMIZE_MAX_BODY_BYTES` (default 2 MiB)
    /// - `SUPPLYMIZE_REQUEST_TIMEOUT_SECONDS` (default 15 seconds)
    pub fn from_env() -> Result<Self, ConfigError> {
        let bind_addr = std::env::var("SUPPLYMIZE_OPTIMIZER_ADDR")
            .unwrap_or_else(|_| DEFAULT_BIND_ADDR.to_owned())
            .parse()
            .map_err(|source| ConfigError::InvalidSocketAddress {
                variable: "SUPPLYMIZE_OPTIMIZER_ADDR",
                source,
            })?;

        let body_limit_bytes =
            parse_env("SUPPLYMIZE_MAX_BODY_BYTES", DEFAULT_BODY_LIMIT_BYTES, 1_024)?;
        let timeout_seconds = parse_env(
            "SUPPLYMIZE_REQUEST_TIMEOUT_SECONDS",
            DEFAULT_REQUEST_TIMEOUT_SECONDS,
            1,
        )?;

        Ok(Self {
            bind_addr,
            body_limit_bytes,
            request_timeout: Duration::from_secs(timeout_seconds),
        })
    }
}

fn parse_env<T>(variable: &'static str, default: T, minimum: T) -> Result<T, ConfigError>
where
    T: std::str::FromStr + PartialOrd + Copy + std::fmt::Display,
{
    let Ok(raw) = std::env::var(variable) else {
        return Ok(default);
    };

    let value = raw
        .parse::<T>()
        .map_err(|_| ConfigError::InvalidNumber { variable })?;
    if value < minimum {
        return Err(ConfigError::BelowMinimum {
            variable,
            minimum: minimum.to_string(),
        });
    }
    Ok(value)
}

#[derive(Debug, Error)]
pub enum ConfigError {
    #[error("{variable} must be a valid socket address")]
    InvalidSocketAddress {
        variable: &'static str,
        #[source]
        source: std::net::AddrParseError,
    },
    #[error("{variable} must be a valid positive integer")]
    InvalidNumber { variable: &'static str },
    #[error("{variable} must be at least {minimum}")]
    BelowMinimum {
        variable: &'static str,
        minimum: String,
    },
}

#[cfg(test)]
mod tests {
    use super::parse_env;

    #[test]
    fn parser_uses_default_when_variable_is_absent() {
        let name = "SUPPLYMIZE_TEST_MISSING_CONFIG_VALUE";
        std::env::remove_var(name);
        assert_eq!(parse_env(name, 42_u64, 1).expect("valid default"), 42);
    }
}
