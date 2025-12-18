package metrics

import (
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func setupPromRegistry(t *testing.T) *prometheus.Registry {
	t.Helper()

	httpRequestsTotal = nil
	registerOnce = sync.Once{}

	reg := prometheus.NewRegistry()
	oldRegisterer := prometheus.DefaultRegisterer
	oldGatherer := prometheus.DefaultGatherer

	prometheus.DefaultRegisterer = reg
	prometheus.DefaultGatherer = reg

	t.Cleanup(func() {
		prometheus.DefaultRegisterer = oldRegisterer
		prometheus.DefaultGatherer = oldGatherer
		httpRequestsTotal = nil
		registerOnce = sync.Once{}
	})

	return reg
}

func getCounterValue(t *testing.T, reg *prometheus.Registry, name string, wantLabels map[string]string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			gotLabels := make(map[string]string, len(m.GetLabel()))
			for _, lp := range m.GetLabel() {
				gotLabels[lp.GetName()] = lp.GetValue()
			}

			match := true
			for k, v := range wantLabels {
				if gotLabels[k] != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}

			if m.GetCounter() == nil {
				t.Fatalf("metric %q is not a counter", name)
			}
			return m.GetCounter().GetValue()
		}
	}

	t.Fatalf("counter %q with labels %+v not found", name, wantLabels)
	return 0
}

func TestIncRequestWithoutRegisterDoesNotPanic(t *testing.T) {
	httpRequestsTotal = nil
	registerOnce = sync.Once{}
	IncRequest("GET", "/health", 200)
}

func TestRegisterAndIncRequest(t *testing.T) {
	reg := setupPromRegistry(t)

	Register()
	Register()

	IncRequest("GET", "/health", 200)
	IncRequest("GET", "/health", 200)

	got := getCounterValue(t, reg, "polling_http_requests_total", map[string]string{
		"method": "GET",
		"path":   "/health",
		"status": "200",
	})
	if got != 2 {
		t.Fatalf("expected counter to be 2, got %v", got)
	}
}
