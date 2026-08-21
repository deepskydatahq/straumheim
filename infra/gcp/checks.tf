check "eu_region" {
  assert {
    condition     = startswith(var.region, "europe-")
    error_message = "Cloud Run and Artifact Registry region must be an approved europe-* region."
  }
}

check "production_cors" {
  assert {
    condition = var.environment != "production" || (
      length(var.cors_allowed_origins) > 0 && !contains(var.cors_allowed_origins, "*")
    )
    error_message = "production requires at least one exact CORS origin and forbids wildcard."
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
