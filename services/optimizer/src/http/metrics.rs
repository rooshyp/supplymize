use std::{
    fmt::Write as _,
    sync::atomic::{AtomicU64, Ordering},
    time::{Duration, Instant},
};

#[derive(Debug)]
pub struct AppMetrics {
    started_at: Instant,
    optimizations_total: AtomicU64,
    optimizations_succeeded: AtomicU64,
    optimizations_failed: AtomicU64,
    optimizations_inflight: AtomicU64,
    solve_time_micros_total: AtomicU64,
}

impl Default for AppMetrics {
    fn default() -> Self {
        Self {
            started_at: Instant::now(),
            optimizations_total: AtomicU64::new(0),
            optimizations_succeeded: AtomicU64::new(0),
            optimizations_failed: AtomicU64::new(0),
            optimizations_inflight: AtomicU64::new(0),
            solve_time_micros_total: AtomicU64::new(0),
        }
    }
}

impl AppMetrics {
    pub(super) fn begin(&self) {
        self.optimizations_total.fetch_add(1, Ordering::Relaxed);
        self.optimizations_inflight.fetch_add(1, Ordering::Relaxed);
    }

    pub(super) fn finish(&self, succeeded: bool, elapsed: Duration) {
        self.optimizations_inflight.fetch_sub(1, Ordering::Relaxed);
        if succeeded {
            self.optimizations_succeeded.fetch_add(1, Ordering::Relaxed);
        } else {
            self.optimizations_failed.fetch_add(1, Ordering::Relaxed);
        }
        let micros = u64::try_from(elapsed.as_micros()).unwrap_or(u64::MAX);
        self.solve_time_micros_total
            .fetch_add(micros, Ordering::Relaxed);
    }

    #[must_use]
    pub fn render_prometheus(&self) -> String {
        let total = self.optimizations_total.load(Ordering::Relaxed);
        let succeeded = self.optimizations_succeeded.load(Ordering::Relaxed);
        let failed = self.optimizations_failed.load(Ordering::Relaxed);
        let inflight = self.optimizations_inflight.load(Ordering::Relaxed);
        let duration_seconds =
            self.solve_time_micros_total.load(Ordering::Relaxed) as f64 / 1_000_000.0;
        let uptime = self.started_at.elapsed().as_secs_f64();

        let mut output = String::with_capacity(1_024);
        writeln!(
            output,
            "# HELP supplymize_optimizer_requests_total Optimization requests received.\n\
             # TYPE supplymize_optimizer_requests_total counter\n\
             supplymize_optimizer_requests_total {total}"
        )
        .expect("writing to a String cannot fail");
        writeln!(
            output,
            "# HELP supplymize_optimizer_requests_succeeded_total Successful optimization requests.\n\
             # TYPE supplymize_optimizer_requests_succeeded_total counter\n\
             supplymize_optimizer_requests_succeeded_total {succeeded}"
        )
        .expect("writing to a String cannot fail");
        writeln!(
            output,
            "# HELP supplymize_optimizer_requests_failed_total Failed optimization requests.\n\
             # TYPE supplymize_optimizer_requests_failed_total counter\n\
             supplymize_optimizer_requests_failed_total {failed}"
        )
        .expect("writing to a String cannot fail");
        writeln!(
            output,
            "# HELP supplymize_optimizer_requests_inflight Optimization requests currently running.\n\
             # TYPE supplymize_optimizer_requests_inflight gauge\n\
             supplymize_optimizer_requests_inflight {inflight}"
        )
        .expect("writing to a String cannot fail");
        writeln!(
            output,
            "# HELP supplymize_optimizer_solve_duration_seconds_total Cumulative optimization wall time.\n\
             # TYPE supplymize_optimizer_solve_duration_seconds_total counter\n\
             supplymize_optimizer_solve_duration_seconds_total {duration_seconds:.6}"
        )
        .expect("writing to a String cannot fail");
        writeln!(
            output,
            "# HELP supplymize_optimizer_uptime_seconds Process uptime.\n\
             # TYPE supplymize_optimizer_uptime_seconds gauge\n\
             supplymize_optimizer_uptime_seconds {uptime:.3}"
        )
        .expect("writing to a String cannot fail");
        output
    }
}
