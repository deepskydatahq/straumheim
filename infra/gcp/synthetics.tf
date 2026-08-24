resource "google_monitoring_uptime_check_config" "collector" {
  count = var.enable_production_canary ? 1 : 0

  display_name       = "${local.prefix}: collector HTTPS health"
  timeout            = "10s"
  period             = "60s"
  selected_regions   = ["EUROPE", "USA", "ASIA_PACIFIC"]
  checker_type       = "STATIC_IP_CHECKERS"
  log_check_failures = true
  user_labels        = local.labels

  http_check {
    path           = "/health"
    port           = 443
    request_method = "GET"
    use_ssl        = true
    validate_ssl   = true
  }

  monitored_resource {
    type = "uptime_url"
    labels = {
      host       = var.collector_domain
      project_id = var.project_id
    }
  }

  depends_on = [google_project_service.required, google_cloud_run_domain_mapping.collector]
}

resource "google_monitoring_alert_policy" "collector_uptime" {
  count = var.enable_production_canary ? 1 : 0

  display_name          = "${local.prefix}: collector HTTPS unavailable"
  combiner              = "OR"
  notification_channels = var.notification_channel_ids

  conditions {
    display_name = "Managed HTTPS health check fails"
    condition_threshold {
      filter = join(" AND ", [
        "resource.type = \"uptime_url\"",
        "metric.type = \"monitoring.googleapis.com/uptime_check/check_passed\"",
        "metric.label.check_id = \"${google_monitoring_uptime_check_config.collector[0].uptime_check_id}\"",
      ])
      comparison      = "COMPARISON_LT"
      threshold_value = 1
      duration        = "120s"
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_FRACTION_TRUE"
      }
    }
  }

  documentation {
    content   = "The production custom-domain HTTPS health check is failing. Verify DNS/certificate/Cloud Run before changing traffic or data resources."
    mime_type = "text/markdown"
  }
}

resource "google_cloud_scheduler_job" "soak_canary" {
  count = var.enable_production_canary ? 1 : 0

  name             = "${local.prefix}-soak-canary"
  description      = "Scheduled production webhook canary during and after the M012 soak"
  region           = var.region
  schedule         = var.soak_canary_schedule
  time_zone        = "Etc/UTC"
  attempt_deadline = "30s"

  http_target {
    uri         = "https://${var.collector_domain}/webhook"
    http_method = "POST"
    headers = {
      "Content-Type" = "application/json"
    }
    body = base64encode(jsonencode({
      event    = "straumheim_production_canary"
      proof_id = "m012-production-soak"
      source   = "cloud-scheduler"
    }))
  }

  retry_config {
    retry_count          = 3
    min_backoff_duration = "5s"
    max_backoff_duration = "60s"
    max_retry_duration   = "300s"
  }

  depends_on = [google_project_service.required, google_cloud_run_domain_mapping.collector]
}
