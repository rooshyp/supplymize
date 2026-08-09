//! SupplyMize's explainable supply-chain optimization engine.
//!
//! The crate intentionally separates transport concerns from the application
//! service and mathematical model so the optimizer can also be embedded in
//! batch workers or exercised directly by tests.

pub mod application;
pub mod config;
pub mod domain;
pub mod http;

pub use application::OptimizerService;
pub use http::{router, AppState};
