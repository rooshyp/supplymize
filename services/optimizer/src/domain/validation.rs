use std::collections::HashSet;

use serde::Serialize;

use super::OptimizationRequest;

const MAX_SUPPLIERS: usize = 500;
const MAX_WAREHOUSES: usize = 500;
const MAX_LANES: usize = 10_000;
const MAX_DEMAND_ROWS: usize = 5_000;
const MAX_UNITS: f64 = 1.0e15;

#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
pub struct ValidationIssue {
    pub field: String,
    pub code: &'static str,
    pub message: String,
}

impl ValidationIssue {
    fn new(field: impl Into<String>, code: &'static str, message: impl Into<String>) -> Self {
        Self {
            field: field.into(),
            code,
            message: message.into(),
        }
    }
}

#[must_use]
pub fn validate_request(request: &OptimizationRequest) -> Vec<ValidationIssue> {
    let mut issues = Vec::new();

    validate_identifier("sku", &request.sku, &mut issues);
    if request.currency.len() != 3
        || !request
            .currency
            .bytes()
            .all(|character| character.is_ascii_uppercase())
    {
        issues.push(ValidationIssue::new(
            "currency",
            "invalid_currency",
            "must be a three-letter uppercase ISO 4217 code",
        ));
    }
    validate_non_negative("lost_sale_penalty", request.lost_sale_penalty, &mut issues);
    validate_non_negative(
        "carbon_price_per_kg",
        request.carbon_price_per_kg,
        &mut issues,
    );
    validate_non_negative(
        "average_storage_days",
        request.average_storage_days,
        &mut issues,
    );

    validate_collection_size(
        "suppliers",
        request.suppliers.len(),
        1,
        MAX_SUPPLIERS,
        &mut issues,
    );
    validate_collection_size(
        "warehouses",
        request.warehouses.len(),
        1,
        MAX_WAREHOUSES,
        &mut issues,
    );
    validate_collection_size("lanes", request.lanes.len(), 0, MAX_LANES, &mut issues);
    validate_collection_size(
        "demand",
        request.demand.len(),
        1,
        MAX_DEMAND_ROWS,
        &mut issues,
    );

    let mut supplier_ids = HashSet::with_capacity(request.suppliers.len());
    for (index, supplier) in request.suppliers.iter().enumerate() {
        let path = format!("suppliers[{index}]");
        validate_identifier(&format!("{path}.id"), &supplier.id, &mut issues);
        if !supplier_ids.insert(supplier.id.as_str()) {
            issues.push(ValidationIssue::new(
                format!("{path}.id"),
                "duplicate_id",
                format!("supplier id '{}' appears more than once", supplier.id),
            ));
        }
        validate_capacity(
            &format!("{path}.capacity_units"),
            supplier.capacity_units,
            &mut issues,
        );
    }

    let mut warehouse_ids = HashSet::with_capacity(request.warehouses.len());
    for (index, warehouse) in request.warehouses.iter().enumerate() {
        let path = format!("warehouses[{index}]");
        validate_identifier(&format!("{path}.id"), &warehouse.id, &mut issues);
        if !warehouse_ids.insert(warehouse.id.as_str()) {
            issues.push(ValidationIssue::new(
                format!("{path}.id"),
                "duplicate_id",
                format!("warehouse id '{}' appears more than once", warehouse.id),
            ));
        }
        validate_capacity(
            &format!("{path}.capacity_units"),
            warehouse.capacity_units,
            &mut issues,
        );
        validate_non_negative(
            &format!("{path}.storage_cost_per_unit_day"),
            warehouse.storage_cost_per_unit_day,
            &mut issues,
        );
    }

    let mut lane_ids = HashSet::with_capacity(request.lanes.len());
    for (index, lane) in request.lanes.iter().enumerate() {
        let path = format!("lanes[{index}]");
        validate_identifier(&format!("{path}.id"), &lane.id, &mut issues);
        if !lane_ids.insert(lane.id.as_str()) {
            issues.push(ValidationIssue::new(
                format!("{path}.id"),
                "duplicate_id",
                format!("lane id '{}' appears more than once", lane.id),
            ));
        }
        if !supplier_ids.contains(lane.supplier_id.as_str()) {
            issues.push(ValidationIssue::new(
                format!("{path}.supplier_id"),
                "unknown_reference",
                format!("supplier '{}' does not exist", lane.supplier_id),
            ));
        }
        if !warehouse_ids.contains(lane.warehouse_id.as_str()) {
            issues.push(ValidationIssue::new(
                format!("{path}.warehouse_id"),
                "unknown_reference",
                format!("warehouse '{}' does not exist", lane.warehouse_id),
            ));
        }
        validate_positive(
            &format!("{path}.distance_km"),
            lane.distance_km,
            &mut issues,
        );
        validate_non_negative(
            &format!("{path}.transport_cost_per_unit_km"),
            lane.transport_cost_per_unit_km,
            &mut issues,
        );
        validate_non_negative(
            &format!("{path}.carbon_kg_per_unit_km"),
            lane.carbon_kg_per_unit_km,
            &mut issues,
        );
        validate_positive(
            &format!("{path}.truck_capacity_units"),
            lane.truck_capacity_units,
            &mut issues,
        );
        if lane.max_trucks == 0 {
            issues.push(ValidationIssue::new(
                format!("{path}.max_trucks"),
                "out_of_range",
                "must be at least 1",
            ));
        }
        let lane_capacity = lane.truck_capacity_units * f64::from(lane.max_trucks);
        if !lane_capacity.is_finite() || lane_capacity > MAX_UNITS {
            issues.push(ValidationIssue::new(
                format!("{path}.max_trucks"),
                "out_of_range",
                format!("combined lane capacity must not exceed {MAX_UNITS}"),
            ));
        }
    }

    let mut total_demand = 0.0;
    for (index, demand) in request.demand.iter().enumerate() {
        let path = format!("demand[{index}]");
        if !warehouse_ids.contains(demand.warehouse_id.as_str()) {
            issues.push(ValidationIssue::new(
                format!("{path}.warehouse_id"),
                "unknown_reference",
                format!("warehouse '{}' does not exist", demand.warehouse_id),
            ));
        }
        validate_positive(&format!("{path}.units"), demand.units, &mut issues);
        total_demand += demand.units;
    }
    if !total_demand.is_finite() || total_demand > MAX_UNITS {
        issues.push(ValidationIssue::new(
            "demand",
            "out_of_range",
            format!("total demand must not exceed {MAX_UNITS}"),
        ));
    }

    issues
}

fn validate_identifier(field: &str, value: &str, issues: &mut Vec<ValidationIssue>) {
    let valid = !value.trim().is_empty()
        && value.len() <= 128
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.'));
    if !valid {
        issues.push(ValidationIssue::new(
            field,
            "invalid_identifier",
            "must be 1-128 characters using letters, numbers, '.', '_' or '-'",
        ));
    }
}

fn validate_collection_size(
    field: &str,
    length: usize,
    minimum: usize,
    maximum: usize,
    issues: &mut Vec<ValidationIssue>,
) {
    if length < minimum || length > maximum {
        issues.push(ValidationIssue::new(
            field,
            "collection_size",
            format!("must contain between {minimum} and {maximum} entries"),
        ));
    }
}

fn validate_capacity(field: &str, value: f64, issues: &mut Vec<ValidationIssue>) {
    validate_non_negative(field, value, issues);
}

fn validate_non_negative(field: &str, value: f64, issues: &mut Vec<ValidationIssue>) {
    if !value.is_finite() || !(0.0..=MAX_UNITS).contains(&value) {
        issues.push(ValidationIssue::new(
            field,
            "out_of_range",
            format!("must be a finite number between 0 and {MAX_UNITS}"),
        ));
    }
}

fn validate_positive(field: &str, value: f64, issues: &mut Vec<ValidationIssue>) {
    if !value.is_finite() || !(0.0..=MAX_UNITS).contains(&value) || value == 0.0 {
        issues.push(ValidationIssue::new(
            field,
            "out_of_range",
            format!("must be a finite number greater than 0 and at most {MAX_UNITS}"),
        ));
    }
}

#[cfg(test)]
mod tests {
    use super::validate_request;
    use crate::domain::{Demand, Lane, OptimizationRequest, SupplierNode, WarehouseNode};

    fn valid_request() -> OptimizationRequest {
        OptimizationRequest {
            sku: "SKU-1".to_owned(),
            suppliers: vec![SupplierNode {
                id: "supplier-1".to_owned(),
                capacity_units: 100.0,
            }],
            warehouses: vec![WarehouseNode {
                id: "warehouse-1".to_owned(),
                capacity_units: 100.0,
                storage_cost_per_unit_day: 0.1,
            }],
            lanes: vec![Lane {
                id: "lane-1".to_owned(),
                supplier_id: "supplier-1".to_owned(),
                warehouse_id: "warehouse-1".to_owned(),
                distance_km: 10.0,
                transport_cost_per_unit_km: 0.02,
                carbon_kg_per_unit_km: 0.001,
                truck_capacity_units: 50.0,
                max_trucks: 2,
            }],
            demand: vec![Demand {
                warehouse_id: "warehouse-1".to_owned(),
                units: 75.0,
            }],
            lost_sale_penalty: 12.0,
            carbon_price_per_kg: 0.2,
            average_storage_days: 1.0,
            currency: "USD".to_owned(),
        }
    }

    #[test]
    fn accepts_a_valid_network() {
        assert!(validate_request(&valid_request()).is_empty());
    }

    #[test]
    fn reports_all_reference_and_numeric_errors() {
        let mut request = valid_request();
        request.suppliers[0].capacity_units = -1.0;
        request.lanes[0].warehouse_id = "missing".to_owned();
        request.demand[0].units = f64::NAN;

        let issues = validate_request(&request);
        assert!(issues.len() >= 3);
        assert!(issues.iter().any(|issue| issue.code == "unknown_reference"));
        assert!(issues.iter().any(|issue| issue.code == "out_of_range"));
    }
}
