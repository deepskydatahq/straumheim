check "eu_region" {
  assert {
    condition     = startswith(var.region, "europe-")
    error_message = "Cloud Run and Artifact Registry region must be an approved europe-* region."
  }
}

check "production_cors" {
  assert {
    condition     = var.environment != "production" || length(var.cors_allowed_origins) > 0
    error_message = "production requires an explicit CORS origin list; use wildcard only with recorded owner approval."
  }
}

check "synthetic_canary_domain" {
  assert {
    condition     = !var.enable_production_canary || var.collector_domain != ""
    error_message = "scheduled canaries require collector_domain."
  }
}

check "production_destroy_safety" {
  assert {
    condition     = var.environment != "production" || !var.delete_proof_data_on_destroy
    error_message = "production cannot enable proof dataset content deletion."
  }
}

check "scaling_bounds" {
  assert {
    condition = (
      var.collector_min_instances >= 0 &&
      var.collector_max_instances >= var.collector_min_instances &&
      var.writer_min_instances >= 0 &&
      var.writer_max_instances >= var.writer_min_instances
    )
    error_message = "maximum instances must be greater than or equal to non-negative minimum instances."
  }
}
