// Package metrics reports the outcome of a vault-manager run to a Prometheus
// Pushgateway.
//
// vault-manager runs as a short-lived Kubernetes CronJob: the process starts,
// does its work, and exits. Prometheus discovers and *scrapes* long-running
// targets, so it can never scrape a job that has already terminated. The
// canonical pattern for batch/cron workloads is instead to *push* the run's
// result metrics to a Pushgateway, which Prometheus then scrapes on its own
// schedule. Each run overwrites the previous sample for its grouping key, so the
// gateway always holds the most recent run's outcome.
//
// Metrics are entirely opt-in: when no Pushgateway URL is configured the
// recorder is a no-op, so the harness runs unchanged in environments without a
// monitoring stack.
package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
)

// Recorder collects the outcome of a single run and pushes it to a Pushgateway.
// A disabled Recorder (see New) is a no-op: every method is safe to call.
type Recorder struct {
	registry *prometheus.Registry
	pusher   *push.Pusher
	enabled  bool

	lastRunTimestamp prometheus.Gauge
	lastRunSuccess   prometheus.Gauge
	runDuration      prometheus.Gauge
	unprocessed      prometheus.Gauge
	archived         prometheus.Gauge
}

// New builds a Recorder that pushes to pushgatewayURL under the given job name,
// labelled by instance (typically the pod/host name so multiple deployments of
// the harness don't clobber each other's samples).
//
// When pushgatewayURL is empty the returned Recorder is disabled and every
// method is a no-op — callers never need to branch on whether metrics are on.
func New(pushgatewayURL, job, instance string) *Recorder {
	r := &Recorder{
		lastRunTimestamp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "vault_manager_last_run_timestamp_seconds",
			Help: "Unix timestamp of the last completed vault-manager run.",
		}),
		lastRunSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "vault_manager_last_run_success",
			Help: "Whether the last vault-manager run succeeded (1) or failed (0).",
		}),
		runDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "vault_manager_run_duration_seconds",
			Help: "Wall-clock duration of the last vault-manager run in seconds.",
		}),
		unprocessed: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "vault_manager_braindumps_unprocessed",
			Help: "Number of unprocessed braindumps found at the start of the run.",
		}),
		archived: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "vault_manager_braindumps_archived",
			Help: "Number of braindumps archived (processed) during the run.",
		}),
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(r.lastRunTimestamp, r.lastRunSuccess, r.runDuration, r.unprocessed, r.archived)
	r.registry = registry

	if pushgatewayURL == "" {
		return r
	}

	r.enabled = true
	r.pusher = push.New(pushgatewayURL, job).
		Grouping("instance", instance).
		Gatherer(registry)
	return r
}

// Enabled reports whether metrics will actually be pushed.
func (r *Recorder) Enabled() bool { return r != nil && r.enabled }

// SetUnprocessed records how many unprocessed braindumps were found.
func (r *Recorder) SetUnprocessed(n int) {
	if r == nil {
		return
	}
	r.unprocessed.Set(float64(n))
}

// SetArchived records how many braindumps were archived this run.
func (r *Recorder) SetArchived(n int) {
	if r == nil {
		return
	}
	r.archived.Set(float64(n))
}

// SetOutcome records the run's success and wall-clock duration, and stamps the
// completion time.
func (r *Recorder) SetOutcome(success bool, duration time.Duration) {
	if r == nil {
		return
	}
	if success {
		r.lastRunSuccess.Set(1)
	} else {
		r.lastRunSuccess.Set(0)
	}
	r.runDuration.Set(duration.Seconds())
	r.lastRunTimestamp.Set(float64(time.Now().Unix()))
}

// Push sends the collected metrics to the Pushgateway. It is a no-op (returns
// nil) when the recorder is disabled.
func (r *Recorder) Push(ctx context.Context) error {
	if !r.Enabled() {
		return nil
	}
	if err := r.pusher.PushContext(ctx); err != nil {
		return fmt.Errorf("push metrics: %w", err)
	}
	return nil
}
