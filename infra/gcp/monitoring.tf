resource "google_monitoring_alert_policy" "collector_5xx" {
  display_name          = "${local.prefix}: collector HTTP 5xx"
  combiner              = "OR"
  notification_channels = var.notification_channel_ids

  conditions {
    display_name = "Collector 5xx responses"
    condition_threshold {
      filter = join(" AND ", [
        "resource.type = \"cloud_run_revision\"",
        "resource.label.service_name = \"${google_cloud_run_v2_service.collector.name}\"",
        "metric.type = \"run.googleapis.com/request_count\"",
        "metric.label.response_code_class = \"5xx\"",
      ])
      comparison      = "COMPARISON_GT"
      threshold_value = 0
      duration        = "60s"
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_RATE"
      }
    }
  }

  documentation {
    content   = "Collector 5xx means durable Pub/Sub acceptance was not acknowledged. Inspect Cloud Run logs and Pub/Sub API health."
    mime_type = "text/markdown"
  }
}

resource "google_monitoring_alert_policy" "writer_5xx" {
  display_name          = "${local.prefix}: writer HTTP 5xx"
  combiner              = "OR"
  notification_channels = var.notification_channel_ids

  conditions {
    display_name = "Writer 5xx responses"
    condition_threshold {
      filter = join(" AND ", [
        "resource.type = \"cloud_run_revision\"",
        "resource.label.service_name = \"${google_cloud_run_v2_service.writer.name}\"",
        "metric.type = \"run.googleapis.com/request_count\"",
        "metric.label.response_code_class = \"5xx\"",
      ])
      comparison      = "COMPARISON_GT"
      threshold_value = 0
      duration        = "60s"
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_RATE"
      }
    }
  }

  documentation {
    content   = "Writer 5xx leaves Pub/Sub messages unacknowledged for redelivery. Inspect writer logs and BigQuery availability."
    mime_type = "text/markdown"
  }
}

resource "google_monitoring_alert_policy" "oldest_unacked" {
  display_name          = "${local.prefix}: oldest unacknowledged event"
  combiner              = "OR"
  notification_channels = var.notification_channel_ids

  conditions {
    display_name = "Oldest event exceeds delivery objective"
    condition_threshold {
      filter = join(" AND ", [
        "resource.type = \"pubsub_subscription\"",
        "resource.label.subscription_id = \"${google_pubsub_subscription.writer.name}\"",
        "metric.type = \"pubsub.googleapis.com/subscription/oldest_unacked_message_age\"",
      ])
      comparison      = "COMPARISON_GT"
      threshold_value = var.oldest_message_age_threshold_seconds
      duration        = "300s"
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_MAX"
      }
    }
  }

  documentation {
    content   = "Delivery is stale. Keep collector live, inspect writer/BigQuery, and watch retention and DLQ limits."
    mime_type = "text/markdown"
  }
}

resource "google_monitoring_alert_policy" "backlog" {
  display_name          = "${local.prefix}: Pub/Sub backlog"
  combiner              = "OR"
  notification_channels = var.notification_channel_ids

  conditions {
    display_name = "Undelivered events exceed capacity threshold"
    condition_threshold {
      filter = join(" AND ", [
        "resource.type = \"pubsub_subscription\"",
        "resource.label.subscription_id = \"${google_pubsub_subscription.writer.name}\"",
        "metric.type = \"pubsub.googleapis.com/subscription/num_undelivered_messages\"",
      ])
      comparison      = "COMPARISON_GT"
      threshold_value = var.backlog_message_threshold
      duration        = "300s"
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_MAX"
      }
    }
  }
}

resource "google_monitoring_alert_policy" "dead_letter" {
  display_name          = "${local.prefix}: dead-letter messages"
  combiner              = "OR"
  notification_channels = var.notification_channel_ids

  conditions {
    display_name = "Dead-letter subscription is non-empty"
    condition_threshold {
      filter = join(" AND ", [
        "resource.type = \"pubsub_subscription\"",
        "resource.label.subscription_id = \"${google_pubsub_subscription.dead_letter.name}\"",
        "metric.type = \"pubsub.googleapis.com/subscription/num_undelivered_messages\"",
      ])
      comparison      = "COMPARISON_GT"
      threshold_value = 0
      duration        = "60s"
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_MAX"
      }
    }
  }

  documentation {
    content   = "A permanent message reached the DLQ. Inspect message metadata and error logs; do not acknowledge until disposition or replay."
    mime_type = "text/markdown"
  }
}
