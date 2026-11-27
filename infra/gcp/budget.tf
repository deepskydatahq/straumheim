data "google_billing_account" "current" {
  count           = var.billing_account_id == "" ? 0 : 1
  billing_account = var.billing_account_id
}

resource "google_billing_budget" "environment" {
  count = var.billing_account_id == "" ? 0 : 1

  billing_account = var.billing_account_id
  display_name    = "${local.prefix} monthly budget"

  budget_filter {
    projects = ["projects/${data.google_project.current.number}"]
  }

  amount {
    specified_amount {
      currency_code = data.google_billing_account.current[0].currency_code
      units         = tostring(var.monthly_budget_amount)
    }
  }

  threshold_rules { threshold_percent = 0.5 }
  threshold_rules { threshold_percent = 0.9 }
  threshold_rules { threshold_percent = 1.0 }
}
