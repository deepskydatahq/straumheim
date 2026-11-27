data "google_project" "current" {
  project_id = var.project_id
}

resource "google_project_service" "required" {
  for_each = local.required_services

  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}

resource "google_artifact_registry_repository" "images" {
  location      = var.region
  repository_id = local.prefix
  description   = "Immutable Straumheim ${var.environment} images"
  format        = "DOCKER"
  labels        = local.labels

  depends_on = [google_project_service.required]
}

resource "google_bigquery_dataset" "events" {
  dataset_id                 = local.dataset_id
  friendly_name              = "Straumheim ${var.environment} events"
  location                   = var.dataset_location
  delete_contents_on_destroy = var.delete_proof_data_on_destroy
  labels                     = local.labels

  depends_on = [google_project_service.required]
}
