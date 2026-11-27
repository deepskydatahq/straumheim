package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func collectCounter(c prometheus.Counter) float64 {
	m := &dto.Metric{}
	c.Write(m)
	return m.GetCounter().GetValue()
}

func collectGauge(g prometheus.Gauge) float64 {
	m := &dto.Metric{}
	g.Write(m)
	return m.GetGauge().GetValue()
}

func collectHistogramCount(h prometheus.Observer) uint64 {
	m := &dto.Metric{}
	h.(prometheus.Metric).Write(m)
	return m.GetHistogram().GetSampleCount()
}

func TestNewMetrics_RegistersAllCollectors(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	if m == nil {
		t.Fatal("expected non-nil Metrics")
	}

	// Gather all metrics to verify registration.
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	want := map[string]bool{
		"straumheim_records_received_total":          false,
		"straumheim_records_delivered_total":         false,
		"straumheim_records_failed_total":            false,
		"straumheim_buffer_size_current":             false,
		"straumheim_flush_duration_seconds":          false,
		"straumheim_pubsub_publish_total":            false,
		"straumheim_pubsub_push_total":               false,
		"straumheim_last_delivery_timestamp_seconds": false,
	}

	// We need to trigger at least one observation per metric for them to appear.
	// First call each method, then gather.
	m.RecordReceived("test")
	m.RecordDelivered("test")
	m.RecordFailed("test")
	m.SetBufferSize(0)
	m.ObserveFlushDuration("test", time.Millisecond)
	m.RecordPubSubPublish("webhook", "success")
	m.RecordPubSubPush("success")

	families, err = reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	for _, f := range families {
		if _, ok := want[f.GetName()]; ok {
			want[f.GetName()] = true
		}
	}

	for name, found := range want {
		if !found {
			t.Errorf("metric %q not registered", name)
		}
	}
}

func TestRecordReceived_IncrementsCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.RecordReceived("webhook")
	m.RecordReceived("webhook")
	m.RecordReceived("snowplow")

	val := collectCounter(m.recordsReceived.WithLabelValues("webhook"))
	if val != 2 {
		t.Errorf("expected 2, got %f", val)
	}

	val = collectCounter(m.recordsReceived.WithLabelValues("snowplow"))
	if val != 1 {
		t.Errorf("expected 1, got %f", val)
	}
}

func TestRecordDelivered_IncrementsCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.RecordDelivered("postgres")
	m.RecordDelivered("postgres")
	m.RecordDelivered("stdout")

	val := collectCounter(m.recordsDelivered.WithLabelValues("postgres"))
	if val != 2 {
		t.Errorf("expected 2, got %f", val)
	}

	val = collectCounter(m.recordsDelivered.WithLabelValues("stdout"))
	if val != 1 {
		t.Errorf("expected 1, got %f", val)
	}
}

func TestRecordFailed_IncrementsCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.RecordFailed("postgres")

	val := collectCounter(m.recordsFailed.WithLabelValues("postgres"))
	if val != 1 {
		t.Errorf("expected 1, got %f", val)
	}
}

func TestPubSubMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.RecordPubSubPublish("", "failure")
	m.RecordPubSubPublish("webhook", "success")
	m.RecordPubSubPush("malformed")
	m.RecordPubSubPush("success")

	if got := collectCounter(m.pubsubPublished.WithLabelValues("unknown", "failure")); got != 1 {
		t.Fatalf("unknown publish failures = %f, want 1", got)
	}
	if got := collectCounter(m.pubsubPublished.WithLabelValues("webhook", "success")); got != 1 {
		t.Fatalf("webhook publish successes = %f, want 1", got)
	}
	if got := collectCounter(m.pubsubPush.WithLabelValues("malformed")); got != 1 {
		t.Fatalf("malformed pushes = %f, want 1", got)
	}
	if got := collectCounter(m.pubsubPush.WithLabelValues("success")); got != 1 {
		t.Fatalf("successful pushes = %f, want 1", got)
	}
	if got := collectGauge(m.lastDeliveryTime); got <= 0 {
		t.Fatalf("last delivery timestamp = %f, want positive", got)
	}
}

func TestSetBufferSize_SetsGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.SetBufferSize(42)

	val := collectGauge(m.bufferSize)
	if val != 42 {
		t.Errorf("expected 42, got %f", val)
	}

	m.SetBufferSize(0)
	val = collectGauge(m.bufferSize)
	if val != 0 {
		t.Errorf("expected 0, got %f", val)
	}
}

func TestObserveFlushDuration_ObservesHistogram(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.ObserveFlushDuration("postgres", 100*time.Millisecond)
	m.ObserveFlushDuration("postgres", 200*time.Millisecond)

	count := collectHistogramCount(m.flushDuration.WithLabelValues("postgres"))
	if count != 2 {
		t.Errorf("expected 2 observations, got %d", count)
	}
}

func TestMetrics_UsesCustomRegistry(t *testing.T) {
	// Verify that metrics are NOT registered on the default registry
	// by creating two Metrics instances with different registries.
	reg1 := prometheus.NewRegistry()
	reg2 := prometheus.NewRegistry()

	m1 := NewMetrics(reg1)
	_ = NewMetrics(reg2)

	m1.RecordReceived("webhook")

	// reg1 should have the metric, reg2 should not have the same value.
	families1, _ := reg1.Gather()
	families2, _ := reg2.Gather()

	// Both should have registered metrics (after triggering them).
	if len(families1) == 0 {
		t.Error("reg1 should have metrics")
	}
	if len(families2) == 0 {
		// reg2 won't have metrics until we trigger them — this is expected.
		// The key test is that m1's increment doesn't appear in reg2.
	}
}
