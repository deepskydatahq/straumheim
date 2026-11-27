resource "google_pubsub_topic" "events" {
  name   = "${local.prefix}-events"
  labels = local.labels

  depends_on = [google_project_service.required]
}

resource "google_pubsub_topic" "dead_letter" {
  name   = "${local.prefix}-dead-letter"
  labels = local.labels

  depends_on = [google_project_service.required]
}

resource "google_pubsub_subscription" "writer" {
  name  = "${local.prefix}-writer"
  topic = google_pubsub_topic.events.id

  ack_deadline_seconds       = 60
  message_retention_duration = var.message_retention_duration
  retain_acked_messages      = false

  retry_policy {
    minimum_backoff = var.retry_minimum_backoff
    maximum_backoff = var.retry_maximum_backoff
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.dead_letter.id
    max_delivery_attempts = var.dead_letter_max_attempts
  }

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.writer.uri}/internal/pubsub/push"
    oidc_token {
      service_account_email = google_service_account.push.email
      audience              = google_cloud_run_v2_service.writer.uri
    }
  }

  expiration_policy {
    ttl = ""
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.push_writer,
    google_service_account_iam_member.pubsub_mints_push_tokens,
    google_pubsub_topic_iam_member.service_agent_forwards_dead_letters,
  ]
}

resource "google_pubsub_subscription" "dead_letter" {
  name                       = "${local.prefix}-dead-letter-inspection"
  topic                      = google_pubsub_topic.dead_letter.id
  ack_deadline_seconds       = 60
  message_retention_duration = var.message_retention_duration
  retain_acked_messages      = false

  expiration_policy {
    ttl = ""
  }
}
