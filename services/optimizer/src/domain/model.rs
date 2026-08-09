use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct OptimizationRequest {
    /// Stock-keeping unit being planned. Each request solves one SKU network.
    pub sku: String,
    #[serde(alias = "supplier_nodes")]
    pub suppliers: Vec<SupplierNode>,
    #[serde(alias = "warehouse_nodes")]
    pub warehouses: Vec<WarehouseNode>,
    pub lanes: Vec<Lane>,
    pub demand: Vec<Demand>,
    /// Opportunity cost for every unit of unfulfilled demand.
    pub lost_sale_penalty: f64,
    /// Internal carbon price used to monetize emitted kilograms of CO2e.
    pub carbon_price_per_kg: f64,
    #[serde(default = "default_storage_days")]
    pub average_storage_days: f64,
    #[serde(default = "default_currency")]
    pub currency: String,
}

fn default_storage_days() -> f64 {
    1.0
}

fn default_currency() -> String {
    "USD".to_owned()
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct SupplierNode {
    pub id: String,
    pub capacity_units: f64,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WarehouseNode {
    pub id: String,
    pub capacity_units: f64,
    pub storage_cost_per_unit_day: f64,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Lane {
    pub id: String,
    pub supplier_id: String,
    pub warehouse_id: String,
    pub distance_km: f64,
    pub transport_cost_per_unit_km: f64,
    pub carbon_kg_per_unit_km: f64,
    pub truck_capacity_units: f64,
    pub max_trucks: u32,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Demand {
    pub warehouse_id: String,
    pub units: f64,
}

#[derive(Clone, Debug, Serialize)]
pub struct OptimizationResponse {
    pub request_id: String,
    pub status: OptimizationStatus,
    pub sku: String,
    pub currency: String,
    pub fulfilled_demand: f64,
    pub unfulfilled_demand: f64,
    pub costs: CostBreakdown,
    pub carbon_kg: f64,
    pub allocations: Vec<Allocation>,
    pub suppliers: Vec<SupplierResult>,
    pub warehouses: Vec<WarehouseResult>,
    pub explanations: Vec<String>,
    pub solver: SolverMetadata,
}

#[derive(Clone, Copy, Debug, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum OptimizationStatus {
    Optimal,
    Partial,
}

#[derive(Clone, Debug, Default, Serialize)]
pub struct CostBreakdown {
    pub transport: f64,
    pub storage: f64,
    pub lost_sales: f64,
    pub carbon: f64,
    pub total: f64,
}

#[derive(Clone, Debug, Serialize)]
pub struct Allocation {
    pub lane_id: String,
    pub supplier_id: String,
    pub warehouse_id: String,
    pub units: f64,
    pub trucks: u64,
    /// Fraction of the lane's configured truck capacity used, from 0.0 to 1.0.
    pub capacity_utilization: f64,
    pub carbon_kg: f64,
    pub costs: CostBreakdown,
}

#[derive(Clone, Debug, Serialize)]
pub struct SupplierResult {
    pub supplier_id: String,
    pub allocated_units: f64,
    pub capacity_units: f64,
    pub capacity_utilization: f64,
    pub binding: bool,
}

#[derive(Clone, Debug, Serialize)]
pub struct WarehouseResult {
    pub warehouse_id: String,
    pub demand_units: f64,
    pub fulfilled_units: f64,
    pub unfulfilled_units: f64,
    pub received_units: f64,
    pub capacity_units: f64,
    pub capacity_utilization: f64,
    pub binding: bool,
}

#[derive(Clone, Debug, Serialize)]
pub struct SolverMetadata {
    pub algorithm: &'static str,
    pub model: &'static str,
    pub solve_time_ms: f64,
    pub globally_optimal: bool,
}
