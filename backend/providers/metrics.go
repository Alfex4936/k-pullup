package providers

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// NewRegistry creates and returns a new Prometheus registry and registers some standard collectors.
func NewRegistry() prometheus.Registerer {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// No need to create a new registry, just use the default one
	requestDurationHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Histogram for HTTP request durations.",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // Use optimized buckets
	})

	// Register only if not already registered
	if err := prometheus.DefaultRegisterer.Register(requestDurationHistogram); err != nil {
		if existing, ok := err.(prometheus.AlreadyRegisteredError); ok {
			requestDurationHistogram = existing.ExistingCollector.(prometheus.Histogram)
		} else {
			panic(err) // Real error, not just a duplicate metric
		}
	}

	return reg
}

// NewLoginCounter creates a custom Prometheus counter for tracking login events.
func NewLoginCounter(registry prometheus.Registerer) prometheus.Counter {
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "login_total",
		Help: "Total number of successful login requests",
	})
	registry.MustRegister(counter)
	return counter
}
