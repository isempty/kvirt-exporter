package collector

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestNewVMCPUCollector(t *testing.T) {
	collector, err := NewVMCPUCollector()
	if err != nil {
		t.Fatalf("Failed to create collector: %v", err)
	}
	if collector == nil {
		t.Fatal("Collector is nil")
	}
}

func TestCollect(t *testing.T) {
	collector, err := NewVMCPUCollector()
	if err != nil {
		t.Fatalf("Failed to create collector: %v", err)
	}

	ch := make(chan prometheus.Metric)
	go func() {
		collector.Collect(ch)
		close(ch)
	}()

	for metric := range ch {
		var m dto.Metric
		if err := metric.Write(&m); err != nil {
			t.Errorf("Failed to write metric: %v", err)
		}
	}
}
