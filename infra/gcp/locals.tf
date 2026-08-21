locals {
  prefix     = "straumheim-${var.environment}"
  dataset_id = var.dataset_id == "" ? "straumheim_${replace(var.environment, "-", "_")}" : var.dataset_id

  required_services = toset([
    "artifactregistry.googleapis.com",
    "bigquery.googleapis.com",
    "bigquerystorage.googleapis.com",
    "billingbudgets.googleapis.com",
    "cloudresourcemanager.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "logging.googleapis.com",
    "monitoring.googleapis.com",
    "pubsub.googleapis.com",
    "run.googleapis.com",
    "secretmanager.googleapis.com",
    "serviceusage.googleapis.com",
  ])

  labels = {
    application = "straumheim"
    environment = var.environment
    managed_by  = "opentofu"
  }

  collector_config = yamlencode({
    runtime = {
      mode = "collector"
      pubsub = {
        project = var.project_id
        topic   = google_pubsub_topic.events.name
      }
    }
    server = {
      host = "0.0.0.0"
      port = 8080
      cors = { allowed_origins = var.cors_allowed_origins }
    }
    inputs = {
      webhook  = { enabled = true, path = "/webhook" }
      pixel    = { enabled = true, path = "/px" }
      snowplow = { enabled = true, path = "/sp" }
    }
  })

  writer_config = yamlencode({
    runtime = {
      mode   = "writer"
      pubsub = { push_path = "/internal/pubsub/push" }
    }
    server = { host = "0.0.0.0", port = 8080 }
    sinks = [{
      name                  = "warehouse"
      type                  = "bigquery"
      mode                  = "batch"
      project               = var.project_id
      dataset               = google_bigquery_dataset.events.dataset_id
      table                 = var.table_id
      location              = var.dataset_location
      max_inflight_requests = 1
    }]
  })
}
