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
// under the License.package centraldogma
package centraldogma

import (
	"testing"

	metrics "github.com/armon/go-metrics"
)

func TestDefaultMetricCollectorConfig(t *testing.T) {
	cnf := DefaultMetricCollectorConfig("dummy")

	if !cnf.EnableServiceLabel {
		t.Fatal()
	}

	if cnf.ServiceName != "dummy" {
		t.Fatal()
	}
}

func TestGlobalPrometheusMetricCollector(t *testing.T) {
	cnf := DefaultMetricCollectorConfig("dummy")

	if _, err := GlobalPrometheusMetricCollector(nil); err != ErrMetricCollectorConfigMustBeSet {
		t.Fatal()
	}

	if _, err := GlobalPrometheusMetricCollector(cnf); err != nil {
		t.Fatal()
	}

	if _, err := GlobalPrometheusMetricCollector(cnf); err != nil {
		t.Fatal()
	}
}

func TestStatsdAndStatsiteMetricCollector(t *testing.T) {
	checker := func(f func(*metrics.Config, string) (*metrics.Metrics, error)) {
		if _, err := f(nil, "127.0.0.1:8080"); err != ErrMetricCollectorConfigMustBeSet {
			t.Fatal()
		}

		cnf := DefaultMetricCollectorConfig("dummy")
		if _, err := f(cnf, "127.0.0.1:8080"); err != nil {
			t.Fatal()
		}
	}

	checker(StatsdMetricCollector)
	checker(StatsiteMetricCollector)
}
