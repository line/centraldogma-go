package centraldogma

import (
	"sync"

	metrics "github.com/armon/go-metrics"
	promMetrics "github.com/armon/go-metrics/prometheus"
)

var metricOnce sync.Once
var globalPrometheusMetricCollector *metrics.Metrics

// DefaultMetricCollectorConfig returns default metric collector config
func DefaultMetricCollectorConfig(name string) (c *metrics.Config) {
	c = metrics.DefaultConfig(name)
	c.EnableServiceLabel = true
	return
}

// GlobalPrometheusMetricCollector returns global metric collector which sinks to Prometheus metrics endpoint.
// Be aware that function may cause panic on error.
func GlobalPrometheusMetricCollector(config *metrics.Config) (m *metrics.Metrics) {
	metricOnce.Do(func() {
		sink, err := promMetrics.NewPrometheusSink()
		if err == nil {
			globalPrometheusMetricCollector, err = metrics.New(config, sink)
		}

		if err != nil {
			panic(err)
		}
	})

	m = globalPrometheusMetricCollector
	return
}

// StatsiteMetricCollector returns metric collector which sinks to statsite endpoint
func StatsiteMetricCollector(config *metrics.Config, addr string) (m *metrics.Metrics, err error) {
	sink, err := metrics.NewStatsiteSink(addr)
	if err != nil {
		return
	}
	m, err = metrics.New(config, sink)
	return
}

// StatsdMetricCollector returns metric collector which sinks to statsd endpoint
func StatsdMetricCollector(config *metrics.Config, addr string) (m *metrics.Metrics, err error) {
	sink, err := metrics.NewStatsdSink(addr)
	if err != nil {
		return
	}
	m, err = metrics.New(config, sink)
	return
}
