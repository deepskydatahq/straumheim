resource "google_cloud_run_domain_mapping" "collector" {
  count = var.collector_domain == "" ? 0 : 1

  location = var.region
  name     = var.collector_domain

  metadata {
    namespace = var.project_id
    labels    = local.labels
  }

  spec {
    route_name = google_cloud_run_v2_service.collector.name
  }
}
