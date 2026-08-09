use std::{
    collections::{BTreeMap, HashMap},
    time::Instant,
};

use thiserror::Error;
use uuid::Uuid;

use super::min_cost_flow::{EdgeRef, MinCostFlow};
use crate::domain::{
    validate_request, Allocation, CostBreakdown, OptimizationRequest, OptimizationResponse,
    OptimizationStatus, SolverMetadata, SupplierResult, ValidationIssue, WarehouseResult,
};

const EPSILON: f64 = 1.0e-8;

#[derive(Clone, Debug, Default)]
pub struct OptimizerService;

impl OptimizerService {
    /// Solves a single-SKU, continuous network-flow model to global optimality.
    ///
    /// The objective includes variable transport, storage, monetized carbon,
    /// and lost-sales costs. Supplier, warehouse, and truck-lane capacities are
    /// hard constraints; unmet demand is modeled as a penalized fallback arc.
    pub fn optimize(
        &self,
        request: OptimizationRequest,
    ) -> Result<OptimizationResponse, OptimizationError> {
        let issues = validate_request(&request);
        if !issues.is_empty() {
            return Err(OptimizationError::Validation(issues));
        }

        let started = Instant::now();
        let mut demand_by_warehouse = BTreeMap::<String, f64>::new();
        for demand in &request.demand {
            *demand_by_warehouse
                .entry(demand.warehouse_id.clone())
                .or_default() += demand.units;
        }
        let demand_rows: Vec<_> = demand_by_warehouse
            .iter()
            .map(|(warehouse_id, units)| (warehouse_id.clone(), *units))
            .collect();

        let supplier_start = 1;
        let warehouse_in_start = supplier_start + request.suppliers.len();
        let warehouse_out_start = warehouse_in_start + request.warehouses.len();
        let demand_start = warehouse_out_start + request.warehouses.len();
        let sink = demand_start + demand_rows.len();
        let source = 0;

        let supplier_nodes: HashMap<_, _> = request
            .suppliers
            .iter()
            .enumerate()
            .map(|(index, supplier)| (supplier.id.as_str(), supplier_start + index))
            .collect();
        let warehouse_indexes: HashMap<_, _> = request
            .warehouses
            .iter()
            .enumerate()
            .map(|(index, warehouse)| (warehouse.id.as_str(), index))
            .collect();

        let mut network = MinCostFlow::new(sink + 1);
        let supplier_edges: Vec<_> = request
            .suppliers
            .iter()
            .enumerate()
            .map(|(index, supplier)| {
                network.add_edge(source, supplier_start + index, supplier.capacity_units, 0.0)
            })
            .collect();

        let lane_edges: Vec<_> = request
            .lanes
            .iter()
            .map(|lane| {
                let supplier_node = supplier_nodes[lane.supplier_id.as_str()];
                let warehouse_index = warehouse_indexes[lane.warehouse_id.as_str()];
                let warehouse = &request.warehouses[warehouse_index];
                let transport_unit_cost = lane.distance_km * lane.transport_cost_per_unit_km;
                let carbon_unit_cost =
                    lane.distance_km * lane.carbon_kg_per_unit_km * request.carbon_price_per_kg;
                let storage_unit_cost =
                    warehouse.storage_cost_per_unit_day * request.average_storage_days;
                let capacity = lane.truck_capacity_units * f64::from(lane.max_trucks);

                network.add_edge(
                    supplier_node,
                    warehouse_in_start + warehouse_index,
                    capacity,
                    transport_unit_cost + carbon_unit_cost + storage_unit_cost,
                )
            })
            .collect();

        let warehouse_capacity_edges: Vec<_> = request
            .warehouses
            .iter()
            .enumerate()
            .map(|(index, warehouse)| {
                network.add_edge(
                    warehouse_in_start + index,
                    warehouse_out_start + index,
                    warehouse.capacity_units,
                    0.0,
                )
            })
            .collect();

        let mut lost_sale_edges = HashMap::<String, EdgeRef>::new();
        for (demand_index, (warehouse_id, units)) in demand_rows.iter().enumerate() {
            let warehouse_index = warehouse_indexes[warehouse_id.as_str()];
            let demand_node = demand_start + demand_index;
            network.add_edge(
                warehouse_out_start + warehouse_index,
                demand_node,
                *units,
                0.0,
            );
            let lost_edge =
                network.add_edge(source, demand_node, *units, request.lost_sale_penalty);
            lost_sale_edges.insert(warehouse_id.clone(), lost_edge);
            network.add_edge(demand_node, sink, *units, 0.0);
        }

        let total_demand: f64 = demand_rows.iter().map(|(_, units)| units).sum();
        network
            .solve(source, sink, total_demand)
            .map_err(|error| OptimizationError::Solver(error.to_string()))?;

        let mut transport_cost = 0.0;
        let mut storage_cost = 0.0;
        let mut carbon_cost = 0.0;
        let mut total_carbon_kg = 0.0;
        let mut allocations = Vec::new();
        let mut binding_constraints = Vec::new();

        for (lane, edge_ref) in request.lanes.iter().zip(lane_edges) {
            let units = network.flow_on(edge_ref);
            let lane_capacity = lane.truck_capacity_units * f64::from(lane.max_trucks);
            if lane_capacity > EPSILON && is_binding(units, lane_capacity) {
                binding_constraints.push(format!(
                    "Lane '{}' is at its {:.4}-unit truck capacity.",
                    lane.id, lane_capacity
                ));
            }
            if units <= EPSILON {
                continue;
            }

            let warehouse_index = warehouse_indexes[lane.warehouse_id.as_str()];
            let warehouse = &request.warehouses[warehouse_index];
            let lane_transport = units * lane.distance_km * lane.transport_cost_per_unit_km;
            let lane_storage =
                units * warehouse.storage_cost_per_unit_day * request.average_storage_days;
            let lane_carbon_kg = units * lane.distance_km * lane.carbon_kg_per_unit_km;
            let lane_carbon_cost = lane_carbon_kg * request.carbon_price_per_kg;

            transport_cost += lane_transport;
            storage_cost += lane_storage;
            carbon_cost += lane_carbon_cost;
            total_carbon_kg += lane_carbon_kg;

            let mut costs = CostBreakdown {
                transport: normalized(lane_transport),
                storage: normalized(lane_storage),
                lost_sales: 0.0,
                carbon: normalized(lane_carbon_cost),
                total: 0.0,
            };
            costs.total = normalized(costs.transport + costs.storage + costs.carbon);
            allocations.push(Allocation {
                lane_id: lane.id.clone(),
                supplier_id: lane.supplier_id.clone(),
                warehouse_id: lane.warehouse_id.clone(),
                units: normalized(units),
                trucks: (units / lane.truck_capacity_units).ceil() as u64,
                capacity_utilization: normalized(ratio(units, lane_capacity)),
                carbon_kg: normalized(lane_carbon_kg),
                costs,
            });
        }

        allocations.sort_by(|left, right| left.lane_id.cmp(&right.lane_id));

        let suppliers = request
            .suppliers
            .iter()
            .zip(supplier_edges)
            .map(|(supplier, edge_ref)| {
                let allocated = network.flow_on(edge_ref);
                let binding = supplier.capacity_units > EPSILON
                    && is_binding(allocated, supplier.capacity_units);
                if binding {
                    binding_constraints.push(format!(
                        "Supplier '{}' is at its {:.4}-unit capacity.",
                        supplier.id, supplier.capacity_units
                    ));
                }
                SupplierResult {
                    supplier_id: supplier.id.clone(),
                    allocated_units: normalized(allocated),
                    capacity_units: normalized(supplier.capacity_units),
                    capacity_utilization: normalized(ratio(allocated, supplier.capacity_units)),
                    binding,
                }
            })
            .collect();

        let mut total_unfulfilled = 0.0;
        let warehouses = request
            .warehouses
            .iter()
            .enumerate()
            .map(|(index, warehouse)| {
                let demand = demand_by_warehouse
                    .get(warehouse.id.as_str())
                    .copied()
                    .unwrap_or(0.0);
                let unfulfilled = lost_sale_edges
                    .get(warehouse.id.as_str())
                    .map_or(0.0, |edge_ref| network.flow_on(*edge_ref));
                let received = network.flow_on(warehouse_capacity_edges[index]);
                let fulfilled = (demand - unfulfilled).max(0.0);
                total_unfulfilled += unfulfilled;
                let binding = warehouse.capacity_units > EPSILON
                    && is_binding(received, warehouse.capacity_units);
                if binding {
                    binding_constraints.push(format!(
                        "Warehouse '{}' is at its {:.4}-unit capacity.",
                        warehouse.id, warehouse.capacity_units
                    ));
                }
                WarehouseResult {
                    warehouse_id: warehouse.id.clone(),
                    demand_units: normalized(demand),
                    fulfilled_units: normalized(fulfilled),
                    unfulfilled_units: normalized(unfulfilled),
                    received_units: normalized(received),
                    capacity_units: normalized(warehouse.capacity_units),
                    capacity_utilization: normalized(ratio(received, warehouse.capacity_units)),
                    binding,
                }
            })
            .collect();

        let total_fulfilled = (total_demand - total_unfulfilled).max(0.0);
        let lost_sales_cost = total_unfulfilled * request.lost_sale_penalty;
        let mut costs = CostBreakdown {
            transport: normalized(transport_cost),
            storage: normalized(storage_cost),
            lost_sales: normalized(lost_sales_cost),
            carbon: normalized(carbon_cost),
            total: 0.0,
        };
        costs.total = normalized(costs.transport + costs.storage + costs.lost_sales + costs.carbon);

        let status = if total_unfulfilled > EPSILON {
            OptimizationStatus::Partial
        } else {
            OptimizationStatus::Optimal
        };
        let mut explanations = vec![
            "Solved a continuous min-cost flow model to global optimality; every unit follows the least-cost feasible path after considering all shared capacities.".to_owned(),
            format!(
                "The objective combines transport, storage for {:.4} average days, lost-sales opportunity cost, and carbon priced at {:.4} {} per kg CO2e.",
                request.average_storage_days, request.carbon_price_per_kg, request.currency
            ),
            "Truck counts are operational estimates rounded up from allocated units; truck capacity and max_trucks jointly impose each lane's hard capacity.".to_owned(),
        ];
        if total_unfulfilled > EPSILON {
            explanations.push(format!(
                "{:.4} units remain unfulfilled because feasible capacity was exhausted or fulfillment's marginal cost exceeded the {:.4} {} lost-sale penalty.",
                normalized(total_unfulfilled), request.lost_sale_penalty, request.currency
            ));
        } else {
            explanations
                .push("All modeled demand is fulfilled within configured capacities.".to_owned());
        }
        if binding_constraints.is_empty() {
            explanations.push(
                "No positive-capacity supplier, warehouse, or lane is fully utilized.".to_owned(),
            );
        } else {
            binding_constraints.sort();
            explanations.extend(binding_constraints);
        }

        Ok(OptimizationResponse {
            request_id: Uuid::new_v4().to_string(),
            status,
            sku: request.sku,
            currency: request.currency,
            fulfilled_demand: normalized(total_fulfilled),
            unfulfilled_demand: normalized(total_unfulfilled),
            costs,
            carbon_kg: normalized(total_carbon_kg),
            allocations,
            suppliers,
            warehouses,
            explanations,
            solver: SolverMetadata {
                algorithm: "successive_shortest_path",
                model: "continuous_capacitated_min_cost_flow",
                solve_time_ms: normalized(started.elapsed().as_secs_f64() * 1_000.0),
                globally_optimal: true,
            },
        })
    }
}

fn ratio(numerator: f64, denominator: f64) -> f64 {
    if denominator <= EPSILON {
        0.0
    } else {
        (numerator / denominator).clamp(0.0, 1.0)
    }
}

fn is_binding(used: f64, capacity: f64) -> bool {
    capacity - used <= EPSILON * capacity.max(1.0)
}

fn normalized(value: f64) -> f64 {
    if value.abs() <= EPSILON {
        0.0
    } else {
        (value * 100_000_000.0).round() / 100_000_000.0
    }
}

#[derive(Debug, Error)]
pub enum OptimizationError {
    #[error("request validation failed")]
    Validation(Vec<ValidationIssue>),
    #[error("optimization solver failed: {0}")]
    Solver(String),
}

#[cfg(test)]
mod tests {
    use super::OptimizerService;
    use crate::domain::{
        Demand, Lane, OptimizationRequest, OptimizationStatus, SupplierNode, WarehouseNode,
    };

    fn request(supplier_capacity: f64, penalty: f64) -> OptimizationRequest {
        OptimizationRequest {
            sku: "FILTER-HEPA-01".to_owned(),
            suppliers: vec![SupplierNode {
                id: "supplier-west".to_owned(),
                capacity_units: supplier_capacity,
            }],
            warehouses: vec![WarehouseNode {
                id: "dc-seattle".to_owned(),
                capacity_units: 100.0,
                storage_cost_per_unit_day: 0.5,
            }],
            lanes: vec![Lane {
                id: "west-to-seattle".to_owned(),
                supplier_id: "supplier-west".to_owned(),
                warehouse_id: "dc-seattle".to_owned(),
                distance_km: 100.0,
                transport_cost_per_unit_km: 0.01,
                carbon_kg_per_unit_km: 0.001,
                truck_capacity_units: 50.0,
                max_trucks: 2,
            }],
            demand: vec![Demand {
                warehouse_id: "dc-seattle".to_owned(),
                units: 100.0,
            }],
            lost_sale_penalty: penalty,
            carbon_price_per_kg: 2.0,
            average_storage_days: 1.0,
            currency: "USD".to_owned(),
        }
    }

    #[test]
    fn honors_capacity_and_returns_an_auditable_cost_breakdown() {
        let response = OptimizerService
            .optimize(request(60.0, 20.0))
            .expect("optimization succeeds");

        assert_eq!(response.status, OptimizationStatus::Partial);
        assert!((response.fulfilled_demand - 60.0).abs() < 1.0e-8);
        assert!((response.unfulfilled_demand - 40.0).abs() < 1.0e-8);
        assert!((response.costs.transport - 60.0).abs() < 1.0e-8);
        assert!((response.costs.storage - 30.0).abs() < 1.0e-8);
        assert!((response.carbon_kg - 6.0).abs() < 1.0e-8);
        assert!((response.costs.carbon - 12.0).abs() < 1.0e-8);
        assert!((response.costs.lost_sales - 800.0).abs() < 1.0e-8);
        assert!((response.costs.total - 902.0).abs() < 1.0e-8);
        assert_eq!(response.allocations[0].trucks, 2);
    }

    #[test]
    fn chooses_lost_sales_when_fulfillment_is_more_expensive() {
        let response = OptimizerService
            .optimize(request(100.0, 1.0))
            .expect("optimization succeeds");

        assert_eq!(response.status, OptimizationStatus::Partial);
        assert!((response.unfulfilled_demand - 100.0).abs() < 1.0e-8);
        assert!(response.allocations.is_empty());
        assert!((response.costs.total - 100.0).abs() < 1.0e-8);
    }

    #[test]
    fn rejects_invalid_references_before_building_the_graph() {
        let mut input = request(100.0, 20.0);
        input.lanes[0].supplier_id = "missing".to_owned();

        let error = OptimizerService
            .optimize(input)
            .expect_err("invalid request must fail");
        assert!(matches!(error, super::OptimizationError::Validation(_)));
    }
}
