// Package metrics provides Prometheus collectors for pipeline observability.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds all Prometheus collectors for pipeline health monitoring.
type Metrics struct {
	recordsReceived  *prometheus.CounterVec
	recordsDelivered *prometheus.CounterVec
	recordsFailed    *prometheus.CounterVec
	bufferSize       prometheus.Gauge
	flushDuration    *prometheus.HistogramVec
	pubsubPublished  *prometheus.CounterVec
	pubsubPush       *prometheus.CounterVec
	lastDeliveryTime prometheus.Gauge
}

// NewMetrics creates a Metrics instance and registers all collectors with reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		recordsReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "straumheim_records_received_total",
			Help: "Total number of records received by protocol.",
		}, []string{"protocol"}),

		recordsDelivered: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "straumheim_records_delivered_total",
			Help: "Total number of records successfully delivered to sinks.",
		}, []string{"sink"}),

		recordsFailed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "straumheim_records_failed_total",
			Help: "Total number of records that failed delivery to sinks.",
		}, []string{"sink"}),

		bufferSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "straumheim_buffer_size_current",
			Help: "Current number of records in the buffer.",
		}),

		flushDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "straumheim_flush_duration_seconds",
			Help:    "Duration of flush operations per sink.",
			Buckets: prometheus.DefBuckets,
		}, []string{"sink"}),

		pubsubPublished: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "straumheim_pubsub_publish_total",
			Help: "Confirmed collector Pub/Sub publish outcomes by protocol and result.",
		}, []string{"protocol", "result"}),

		pubsubPush: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "straumheim_pubsub_push_total",
			Help: "Pub/Sub push processing outcomes by result.",
		}, []string{"result"}),

		lastDeliveryTime: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "straumheim_last_delivery_timestamp_seconds",
			Help: "Unix timestamp of the most recent confirmed request-scoped destination delivery.",
		}),
	}

	reg.MustRegister(m.recordsReceived)
	reg.MustRegister(m.recordsDelivered)
	reg.MustRegister(m.recordsFailed)
	reg.MustRegister(m.bufferSize)
	reg.MustRegister(m.flushDuration)
	reg.MustRegister(m.pubsubPublished)
	reg.MustRegister(m.pubsubPush)
	reg.MustRegister(m.lastDeliveryTime)

	return m
}

// RecordReceived increments the records received counter for the given protocol.
func (m *Metrics) RecordReceived(protocol string) {
	m.recordsReceived.WithLabelValues(protocol).Inc()
}

// RecordDelivered increments the records delivered counter for the given sink.
func (m *Metrics) RecordDelivered(sink string) {
	m.recordsDelivered.WithLabelValues(sink).Inc()
}

// RecordFailed increments the records failed counter for the given sink.
func (m *Metrics) RecordFailed(sink string) {
	m.recordsFailed.WithLabelValues(sink).Inc()
}

// SetBufferSize sets the current buffer size gauge.
func (m *Metrics) SetBufferSize(n int) {
	m.bufferSize.Set(float64(n))
}

// ObserveFlushDuration records a flush duration observation for the given sink.
func (m *Metrics) ObserveFlushDuration(sink string, d time.Duration) {
	m.flushDuration.WithLabelValues(sink).Observe(d.Seconds())
}

// RecordPubSubPublish records one canonical Record publish outcome.
func (m *Metrics) RecordPubSubPublish(protocol, result string) {
	if protocol == "" {
		protocol = "unknown"
	}
	m.pubsubPublished.WithLabelValues(protocol, result).Inc()
}

// RecordPubSubPush records one push request processing outcome.
func (m *Metrics) RecordPubSubPush(result string) {
	m.pubsubPush.WithLabelValues(result).Inc()
	if result == "success" {
		m.lastDeliveryTime.SetToCurrentTime()
	}
}
