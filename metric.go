// Copyright 2019 LINE Corporation
//
// LINE Corporation licenses this file to you under the Apache License,
// version 2.0 (the "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at:
//
//   https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.
package centraldogma

import (
	"sync"

	metrics "github.com/armon/go-metrics"
	promMetrics "github.com/armon/go-metrics/prometheus"
)

var metricOnce sync.Once
var globalPrometheusMetricCollector *metrics.Metrics

// DefaultMetricCollectorConfig returns default metric collector config.
func DefaultMetricCollectorConfig(name string) (c *metrics.Config) {
	c = metrics.DefaultConfig(name)
	c.EnableServiceLabel = true
	return
}

// GlobalPrometheusMetricCollector returns global metric collector which sinks to Prometheus metrics endpoint.
// Be aware that function may cause panic on error.
func GlobalPrometheusMetricCollector(config *metrics.Config) (m *metrics.Metrics, err error) {
	if config == nil {
		err = ErrMetricCollectorConfigMustBeSet
		return
	}

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

// StatsiteMetricCollector returns metric collector which sinks to statsite endpoint.
func StatsiteMetricCollector(config *metrics.Config, addr string) (m *metrics.Metrics, err error) {
	// validate config
	if config == nil {
		err = ErrMetricCollectorConfigMustBeSet
		return
	}

	sink, err := metrics.NewStatsiteSink(addr)
	if err != nil {
		return
	}
	m, err = metrics.New(config, sink)
	return
}

// StatsdMetricCollector returns metric collector which sinks to statsd endpoint.
func StatsdMetricCollector(config *metrics.Config, addr string) (m *metrics.Metrics, err error) {
	// validate config
	if config == nil {
		err = ErrMetricCollectorConfigMustBeSet
		return
	}

	sink, err := metrics.NewStatsdSink(addr)
	if err != nil {
		return
	}
	m, err = metrics.New(config, sink)
	return
}
