resource "google_service_account" "collector" {
  account_id   = "${local.prefix}-collector"
  display_name = "Straumheim ${var.environment} collector"
}

resource "google_service_account" "writer" {
  account_id   = "${local.prefix}-writer"
  display_name = "Straumheim ${var.environment} BigQuery writer"
}

resource "google_service_account" "push" {
  account_id   = "${local.prefix}-push"
  display_name = "Straumheim ${var.environment} Pub/Sub push identity"
}

resource "google_pubsub_topic_iam_member" "collector_publish" {
  topic  = google_pubsub_topic.events.name
  role   = "roles/pubsub.publisher"
  member = "serviceAccount:${google_service_account.collector.email}"
}

resource "google_bigquery_dataset_iam_member" "writer_data_editor" {
  dataset_id = google_bigquery_dataset.events.dataset_id
  role       = "roles/bigquery.dataEditor"
  member     = "serviceAccount:${google_service_account.writer.email}"
}

resource "google_cloud_run_v2_service_iam_member" "push_writer" {
  project  = var.project_id
  location = google_cloud_run_v2_service.writer.location
  name     = google_cloud_run_v2_service.writer.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.push.email}"
}

resource "google_service_account_iam_member" "pubsub_mints_push_tokens" {
  service_account_id = google_service_account.push.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:service-${data.google_project.current.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_pubsub_topic_iam_member" "service_agent_forwards_dead_letters" {
  topic  = google_pubsub_topic.dead_letter.name
  role   = "roles/pubsub.publisher"
  member = "serviceAccount:service-${data.google_project.current.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_pubsub_subscription_iam_member" "service_agent_reads_for_dead_letter" {
  subscription = google_pubsub_subscription.writer.name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:service-${data.google_project.current.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}
