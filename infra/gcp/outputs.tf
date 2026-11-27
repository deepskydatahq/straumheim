output "collector_url" {
  value = google_cloud_run_v2_service.collector.uri
}

output "collector_domain_status" {
  value = var.collector_domain == "" ? null : google_cloud_run_domain_mapping.collector[0].status
}

output "writer_url" {
  value     = google_cloud_run_v2_service.writer.uri
  sensitive = true
}

output "topic" {
  value = google_pubsub_topic.events.id
}

output "subscription" {
  value = google_pubsub_subscription.writer.id
}

output "dead_letter_subscription" {
  value = google_pubsub_subscription.dead_letter.id
}

output "dataset" {
  value = google_bigquery_dataset.events.dataset_id
}

output "runtime_service_accounts" {
  value = {
    collector = google_service_account.collector.email
    writer    = google_service_account.writer.email
    push      = google_service_account.push.email
  }
}
