package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	jobDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "whatever_job_duration_seconds",
		Help:    "How long jobs took, by type (process = transcode) and result.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 16), // 1s … ~9h
	}, []string{"type", "result"})
	jobsRunning = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "whatever_jobs_running",
		Help: "Jobs this worker is running right now.",
	}, []string{"type"})
	hwEncoder = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "whatever_hw_encoder",
		Help: "The video encoder in use (1 for the active one).",
	}, []string{"encoder"})
)

// queueCollector reports the jobs table by type and status at scrape time,
// so the queue length is exact even with several API writers.
type queueCollector struct {
	pool *pgxpool.Pool
	desc *prometheus.Desc
}

func newQueueCollector(pool *pgxpool.Pool) *queueCollector {
	return &queueCollector{pool: pool, desc: prometheus.NewDesc(
		"whatever_jobs", "Jobs in the queue table by type and status (queued = queue length).",
		[]string{"type", "status"}, nil)}
}

func (c *queueCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

func (c *queueCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows, err := c.pool.Query(ctx, `SELECT type, status, count(*) FROM jobs GROUP BY 1, 2`)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.desc, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var typ, status string
		var n int64
		if err := rows.Scan(&typ, &status, &n); err != nil {
			ch <- prometheus.NewInvalidMetric(c.desc, err)
			return
		}
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(n), typ, status)
	}
}

// serveHealth runs /healthz (200 while the database answers) and /metrics.
func serveHealth(ctx context.Context, addr string, pool *pgxpool.Pool) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(jobDuration, jobsRunning, hwEncoder, newQueueCollector(pool),
		prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		pctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(pctx); err != nil {
			http.Error(w, "database: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	log.Printf("worker health and metrics on %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("health server: %v", err)
	}
}

// healthcheck is `movie-worker healthcheck`: the container healthcheck,
// since the worker image has no curl or wget.
func healthcheck(addr string) {
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	c := http.Client{Timeout: 4 * time.Second}
	res, err := c.Get("http://" + addr + "/healthz")
	if err == nil {
		res.Body.Close()
		if res.StatusCode == http.StatusOK {
			os.Exit(0)
		}
		err = fmt.Errorf("status %d", res.StatusCode)
	}
	fmt.Fprintln(os.Stderr, "unhealthy:", err)
	os.Exit(1)
}
