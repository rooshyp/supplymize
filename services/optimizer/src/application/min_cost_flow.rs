use std::{cmp::Ordering, collections::BinaryHeap};

use thiserror::Error;

const EPSILON: f64 = 1.0e-9;

#[derive(Clone, Copy, Debug)]
pub(super) struct EdgeRef {
    from: usize,
    index: usize,
}

#[derive(Clone, Debug)]
struct Edge {
    to: usize,
    reverse: usize,
    capacity: f64,
    initial_capacity: f64,
    cost: f64,
}

/// Successive-shortest-path solver for a continuous capacitated min-cost flow.
///
/// Reduced-cost node potentials make each augmentation a Dijkstra pass while
/// reverse residual edges preserve global optimality.
#[derive(Debug)]
pub(super) struct MinCostFlow {
    graph: Vec<Vec<Edge>>,
}

impl MinCostFlow {
    pub(super) fn new(node_count: usize) -> Self {
        Self {
            graph: vec![Vec::new(); node_count],
        }
    }

    pub(super) fn add_edge(&mut self, from: usize, to: usize, capacity: f64, cost: f64) -> EdgeRef {
        let forward_index = self.graph[from].len();
        let reverse_index = self.graph[to].len();

        self.graph[from].push(Edge {
            to,
            reverse: reverse_index,
            capacity,
            initial_capacity: capacity,
            cost,
        });
        self.graph[to].push(Edge {
            to: from,
            reverse: forward_index,
            capacity: 0.0,
            initial_capacity: 0.0,
            cost: -cost,
        });

        EdgeRef {
            from,
            index: forward_index,
        }
    }

    pub(super) fn flow_on(&self, edge_ref: EdgeRef) -> f64 {
        let edge = &self.graph[edge_ref.from][edge_ref.index];
        (edge.initial_capacity - edge.capacity).max(0.0)
    }

    pub(super) fn solve(
        &mut self,
        source: usize,
        sink: usize,
        required_flow: f64,
    ) -> Result<f64, SolverError> {
        let node_count = self.graph.len();
        let mut potentials = vec![0.0; node_count];
        let mut delivered_flow = 0.0;
        let mut total_cost = 0.0;

        while delivered_flow + EPSILON < required_flow {
            let mut distances = vec![f64::INFINITY; node_count];
            let mut previous = vec![None; node_count];
            let mut queue = BinaryHeap::new();
            distances[source] = 0.0;
            queue.push(QueueState {
                node: source,
                cost: 0.0,
            });

            while let Some(QueueState { node, cost }) = queue.pop() {
                if cost > distances[node] {
                    continue;
                }

                for (edge_index, edge) in self.graph[node].iter().enumerate() {
                    if edge.capacity <= EPSILON {
                        continue;
                    }

                    // Reduced costs are non-negative; max absorbs tiny floating
                    // point drift introduced by prior potential updates.
                    let reduced_cost =
                        (edge.cost + potentials[node] - potentials[edge.to]).max(0.0);
                    let candidate = distances[node] + reduced_cost;
                    if candidate < distances[edge.to] {
                        distances[edge.to] = candidate;
                        previous[edge.to] = Some((node, edge_index));
                        queue.push(QueueState {
                            node: edge.to,
                            cost: candidate,
                        });
                    }
                }
            }

            if !distances[sink].is_finite() {
                return Err(SolverError::InsufficientResidualCapacity {
                    delivered: delivered_flow,
                    required: required_flow,
                });
            }

            for (node, distance) in distances.iter().copied().enumerate() {
                if distance.is_finite() {
                    potentials[node] += distance;
                }
            }

            let mut augmentation = required_flow - delivered_flow;
            let mut cursor = sink;
            while cursor != source {
                let (previous_node, edge_index) =
                    previous[cursor].ok_or(SolverError::BrokenPredecessorChain)?;
                augmentation = augmentation.min(self.graph[previous_node][edge_index].capacity);
                cursor = previous_node;
            }

            if augmentation <= EPSILON {
                return Err(SolverError::ZeroAugmentation);
            }

            cursor = sink;
            while cursor != source {
                let (previous_node, edge_index) =
                    previous[cursor].ok_or(SolverError::BrokenPredecessorChain)?;
                let edge_cost = self.graph[previous_node][edge_index].cost;
                let reverse_index = self.graph[previous_node][edge_index].reverse;

                self.graph[previous_node][edge_index].capacity -= augmentation;
                self.graph[cursor][reverse_index].capacity += augmentation;
                total_cost += augmentation * edge_cost;
                cursor = previous_node;
            }

            delivered_flow += augmentation;
        }

        Ok(total_cost)
    }
}

#[derive(Clone, Copy, Debug)]
struct QueueState {
    node: usize,
    cost: f64,
}

impl PartialEq for QueueState {
    fn eq(&self, other: &Self) -> bool {
        self.node == other.node && self.cost.total_cmp(&other.cost) == Ordering::Equal
    }
}

impl Eq for QueueState {}

impl PartialOrd for QueueState {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for QueueState {
    fn cmp(&self, other: &Self) -> Ordering {
        // BinaryHeap is a max-heap, so reverse the cost comparison.
        other
            .cost
            .total_cmp(&self.cost)
            .then_with(|| other.node.cmp(&self.node))
    }
}

#[derive(Debug, Error)]
pub(super) enum SolverError {
    #[error("network can deliver only {delivered} of {required} required units")]
    InsufficientResidualCapacity { delivered: f64, required: f64 },
    #[error("solver produced an incomplete predecessor chain")]
    BrokenPredecessorChain,
    #[error("solver produced a zero-capacity augmentation")]
    ZeroAugmentation,
}

#[cfg(test)]
mod tests {
    use super::MinCostFlow;

    #[test]
    fn finds_global_minimum_across_competing_paths() {
        // source -> suppliers -> destination -> sink
        let mut network = MinCostFlow::new(5);
        network.add_edge(0, 1, 4.0, 0.0);
        network.add_edge(0, 2, 6.0, 0.0);
        let expensive = network.add_edge(1, 3, 4.0, 3.0);
        let cheap = network.add_edge(2, 3, 6.0, 1.0);
        network.add_edge(3, 4, 8.0, 0.0);

        let cost = network.solve(0, 4, 8.0).expect("feasible flow");

        assert!((cost - 12.0).abs() < 1.0e-8);
        assert!((network.flow_on(cheap) - 6.0).abs() < 1.0e-8);
        assert!((network.flow_on(expensive) - 2.0).abs() < 1.0e-8);
    }

    #[test]
    fn reports_an_infeasible_flow() {
        let mut network = MinCostFlow::new(2);
        network.add_edge(0, 1, 2.0, 0.0);
        assert!(network.solve(0, 1, 3.0).is_err());
    }

    #[test]
    fn uses_residual_edges_to_correct_an_earlier_allocation() {
        // The first zero-cost assignment A -> W1 must be displaced so B can
        // use W1 and A can use W2. A cheapest-edge greedy strategy costs 100;
        // the residual min-cost solution costs 2.
        let mut network = MinCostFlow::new(6);
        network.add_edge(0, 1, 1.0, 0.0); // source -> A
        network.add_edge(0, 2, 1.0, 0.0); // source -> B
        let a_w1 = network.add_edge(1, 3, 1.0, 0.0);
        let a_w2 = network.add_edge(1, 4, 1.0, 1.0);
        let b_w1 = network.add_edge(2, 3, 1.0, 1.0);
        let b_w2 = network.add_edge(2, 4, 1.0, 100.0);
        network.add_edge(3, 5, 1.0, 0.0);
        network.add_edge(4, 5, 1.0, 0.0);

        let cost = network.solve(0, 5, 2.0).expect("feasible flow");

        assert!((cost - 2.0).abs() < 1.0e-8);
        assert!(network.flow_on(a_w1) < 1.0e-8);
        assert!((network.flow_on(a_w2) - 1.0).abs() < 1.0e-8);
        assert!((network.flow_on(b_w1) - 1.0).abs() < 1.0e-8);
        assert!(network.flow_on(b_w2) < 1.0e-8);
    }
}
