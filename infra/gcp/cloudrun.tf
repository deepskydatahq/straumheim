resource "google_cloud_run_v2_service" "collector" {
  name                 = "${local.prefix}-collector"
  location             = var.region
  deletion_protection  = false
  ingress              = "INGRESS_TRAFFIC_ALL"
  invoker_iam_disabled = true
  labels               = local.labels

  lifecycle {
    ignore_changes = [scaling]
  }

  template {
    service_account = google_service_account.collector.email
    timeout         = "60s"

    scaling {
      min_instance_count = var.collector_min_instances
      max_instance_count = var.collector_max_instances
    }

    containers {
      image = var.image

      env {
        name  = "STRAUMHEIM_CONFIG"
        value = "/etc/straumheim/config.yaml"
      }

      ports {
        container_port = 8080
      }

      resources {
        limits   = { cpu = "1", memory = "512Mi" }
        cpu_idle = true
      }

      volume_mounts {
        name       = "config"
        mount_path = "/etc/straumheim"
      }

      startup_probe {
        initial_delay_seconds = 0
        timeout_seconds       = 3
        period_seconds        = 3
        failure_threshold     = 20
        http_get {
          path = "/health"
          port = 8080
        }
      }

      liveness_probe {
        timeout_seconds   = 3
        period_seconds    = 10
        failure_threshold = 3
        http_get {
          path = "/health"
          port = 8080
        }
      }
    }

    volumes {
      name = "config"
      secret {
        secret = google_secret_manager_secret.collector_config.secret_id
        items {
          version = "latest"
          path    = "config.yaml"
        }
      }
    }
  }

  depends_on = [
    google_project_service.required,
    google_secret_manager_secret_iam_member.collector_reads_config,
  ]
}

resource "google_cloud_run_v2_service" "writer" {
  name                = "${local.prefix}-writer"
  location            = var.region
  deletion_protection = false
  ingress             = "INGRESS_TRAFFIC_ALL"
  labels              = local.labels

  lifecycle {
    ignore_changes = [scaling]
  }

  template {
    service_account = google_service_account.writer.email
    timeout         = "60s"

    scaling {
      min_instance_count = var.writer_min_instances
      max_instance_count = var.writer_max_instances
    }

    containers {
      image = var.image

      env {
        name  = "STRAUMHEIM_CONFIG"
        value = "/etc/straumheim/config.yaml"
      }

      ports {
        container_port = 8080
      }

      resources {
        limits   = { cpu = "1", memory = "512Mi" }
        cpu_idle = true
      }

      volume_mounts {
        name       = "config"
        mount_path = "/etc/straumheim"
      }

      startup_probe {
        timeout_seconds   = 3
        period_seconds    = 3
        failure_threshold = 20
        http_get {
          path = "/health"
          port = 8080
        }
      }

      liveness_probe {
        timeout_seconds   = 3
        period_seconds    = 10
        failure_threshold = 3
        http_get {
          path = "/health"
          port = 8080
        }
      }
    }

    volumes {
      name = "config"
      secret {
        secret = google_secret_manager_secret.writer_config.secret_id
        items {
          version = "latest"
          path    = "config.yaml"
        }
      }
    }
  }

  depends_on = [
    google_project_service.required,
    google_secret_manager_secret_iam_member.writer_reads_config,
    google_bigquery_dataset_iam_member.writer_data_editor,
  ]
}
