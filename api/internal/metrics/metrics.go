package metrics

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	Requests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "yuva_http_requests_total", Help: "HTTP requests by method and status.",
	}, []string{"method", "status"})
	RequestSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "yuva_http_request_seconds", Help: "HTTP request latency.", Buckets: prometheus.DefBuckets,
	}, []string{"method"})
)

func Serve(ctx context.Context, addr string, log *slog.Logger) {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("metrics listener", slog.Any("error", err))
	}
}
