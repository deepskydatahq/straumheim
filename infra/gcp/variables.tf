variable "project_id" {
  description = "GCP project that owns the environment."
  type        = string
}

variable "region" {
  description = "EU Cloud Run and Artifact Registry region."
  type        = string
  default     = "europe-west1"
}

variable "environment" {
  description = "Isolated resource-name suffix, for example proof or production."
  type        = string
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,20}$", var.environment))
    error_message = "environment must be a short lowercase resource-name suffix."
  }
}

variable "image" {
  description = "Immutable Artifact Registry image reference."
  type        = string
  validation {
    condition     = can(regex("@sha256:[0-9a-f]{64}$", var.image))
    error_message = "image must be pinned by sha256 digest."
  }
}

variable "dataset_id" {
  description = "BigQuery dataset ID."
  type        = string
  default     = "straumheim"
}

variable "dataset_location" {
  description = "BigQuery dataset location."
  type        = string
  default     = "EU"
}

variable "table_id" {
  description = "BigQuery events table created by the writer when absent."
  type        = string
  default     = "events"
}

variable "cors_allowed_origins" {
  description = "Exact collector CORS origins. Never use wildcard in production."
  type        = list(string)
  default     = []
}

variable "collector_min_instances" {
  type    = number
  default = 2
}

variable "collector_max_instances" {
  type    = number
  default = 10
}

variable "writer_min_instances" {
  type    = number
  default = 0
}

variable "writer_max_instances" {
  type    = number
  default = 10
}

variable "message_retention_duration" {
  type    = string
  default = "604800s"
}

variable "retry_minimum_backoff" {
  type    = string
  default = "10s"
}

variable "retry_maximum_backoff" {
  type    = string
  default = "600s"
}

variable "dead_letter_max_attempts" {
  type    = number
  default = 10
  validation {
    condition     = var.dead_letter_max_attempts >= 5 && var.dead_letter_max_attempts <= 100
    error_message = "Pub/Sub dead-letter attempts must be between 5 and 100."
  }
}

variable "notification_channel_ids" {
  description = "Existing Cloud Monitoring notification channel resource IDs."
  type        = list(string)
  default     = []
}

variable "backlog_message_threshold" {
  type    = number
  default = 1000
}

variable "oldest_message_age_threshold_seconds" {
  type    = number
  default = 300
}

variable "billing_account_id" {
  description = "Optional billing account ID for a project budget."
  type        = string
  default     = ""
}

variable "monthly_budget_amount" {
  type    = number
  default = 25
}

variable "delete_proof_data_on_destroy" {
  description = "Allow dataset contents deletion only for explicitly disposable environments."
  type        = bool
  default     = false
}
