package httpapi

import (
	"context"
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// DomainCollector implements prometheus.Collector. On each scrape it queries
// the database for current queue depth and runner counts.
type DomainCollector struct {
	db *sql.DB

	runsPending   *prometheus.Desc
	runnersOnline *prometheus.Desc
}

// NewDomainCollector creates a collector that queries db on every Prometheus scrape.
func NewDomainCollector(db *sql.DB) *DomainCollector {
	return &DomainCollector{
		db: db,
		runsPending: prometheus.NewDesc(
			"minitower_runs_pending",
			"Number of queued runs by team, app, and environment.",
			[]string{"team", "app", "environment"},
			nil,
		),
		runnersOnline: prometheus.NewDesc(
			"minitower_runners_online",
			"Number of online runners by environment.",
			[]string{"environment"},
			nil,
		),
	}
}

func (c *DomainCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.runsPending
	ch <- c.runnersOnline
}

func (c *DomainCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c.collectRunsPending(ctx, ch)
	c.collectRunnersOnline(ctx, ch)
}

func (c *DomainCollector) collectRunsPending(ctx context.Context, ch chan<- prometheus.Metric) {
	rows, err := c.db.QueryContext(ctx,
		`SELECT t.slug, a.slug, e.name, COUNT(*)
		 FROM runs r
		 JOIN apps a ON r.app_id = a.id
		 JOIN teams t ON r.team_id = t.id
		 JOIN environments e ON r.environment_id = e.id
		 WHERE r.status = 'queued'
		 GROUP BY t.slug, a.slug, e.name`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var team, app, env string
		var count float64
		if err := rows.Scan(&team, &app, &env, &count); err != nil {
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.runsPending, prometheus.GaugeValue, count, team, app, env)
	}
}

func (c *DomainCollector) collectRunnersOnline(ctx context.Context, ch chan<- prometheus.Metric) {
	rows, err := c.db.QueryContext(ctx,
		`SELECT environment, COUNT(*)
		 FROM runners
		 WHERE status = 'online'
		 GROUP BY environment`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var env string
		var count float64
		if err := rows.Scan(&env, &count); err != nil {
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.runnersOnline, prometheus.GaugeValue, count, env)
	}
}
