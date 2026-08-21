resource "google_secret_manager_secret" "collector_config" {
  secret_id = "${local.prefix}-collector-config"
  replication {
    auto {}
  }
  labels = local.labels

  depends_on = [google_project_service.required]
}

resource "google_secret_manager_secret_version" "collector_config" {
  secret      = google_secret_manager_secret.collector_config.id
  secret_data = local.collector_config
}

resource "google_secret_manager_secret_iam_member" "collector_reads_config" {
  secret_id = google_secret_manager_secret.collector_config.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.collector.email}"
}

resource "google_secret_manager_secret" "writer_config" {
  secret_id = "${local.prefix}-writer-config"
  replication {
    auto {}
  }
  labels = local.labels

  depends_on = [google_project_service.required]
}

resource "google_secret_manager_secret_version" "writer_config" {
  secret      = google_secret_manager_secret.writer_config.id
  secret_data = local.writer_config
}

resource "google_secret_manager_secret_iam_member" "writer_reads_config" {
  secret_id = google_secret_manager_secret.writer_config.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.writer.email}"
}
