# GCP-native Straumheim infrastructure

This OpenTofu root provisions the optional M012 profile:

```text
public Cloud Run collector -> Pub/Sub -> private Cloud Run writer -> BigQuery
                                      -> dead-letter topic/subscription
```

It also creates runtime identities, config secrets, Artifact Registry, Cloud Monitoring policies, and an optional budget. No service-account key is created.

## Prerequisites

- OpenTofu 1.12+
- `gcloud` authenticated as a bootstrap/project owner
- an approved EU GCP project and billing account
- a pre-created GCS state bucket with versioning and uniform access
- an existing Monitoring notification channel for production alerts

Never commit `.tfvars`, state, plans, Google application credentials, or GitHub secrets. The repository ignores these files. OpenTofu state contains generated non-secret YAML configuration and must still be access-controlled.

## One-time bootstrap

Choose an environment (`proof` or `production`) and EU region:

```bash
export PROJECT_ID="your-approved-project"
export REGION="europe-west1"
export ENVIRONMENT="proof"
export STATE_BUCKET="${PROJECT_ID}-straumheim-tofu"

gcloud config set project "$PROJECT_ID"
gcloud services enable serviceusage.googleapis.com cloudresourcemanager.googleapis.com

gcloud storage buckets create "gs://${STATE_BUCKET}" \
  --project="$PROJECT_ID" --location=EU --uniform-bucket-level-access
gcloud storage buckets update "gs://${STATE_BUCKET}" --versioning

cp infra/gcp/terraform.tfvars.example infra/gcp/${ENVIRONMENT}.tfvars
# Review every value. The placeholder image must still satisfy @sha256 validation.

tofu -chdir=infra/gcp init \
  -backend-config="bucket=${STATE_BUCKET}" \
  -backend-config="prefix=straumheim/${ENVIRONMENT}"

# Bootstrap APIs and the empty image repository before CI's first push.
tofu -chdir=infra/gcp apply \
  -var-file="${ENVIRONMENT}.tfvars" \
  -target=google_project_service.required \
  -target=google_artifact_registry_repository.images
```

Targeted apply is bootstrap-only. Every normal deployment uses a complete plan.

## Keyless GitHub deployment identity

Create a dedicated deployment service account and a GitHub Workload Identity Federation pool/provider. Restrict the provider attribute condition to this repository and, for production, use a protected GitHub Environment requiring approval.

The deploy identity needs owner-granted provisioning permissions for the resources in this root, typically:

- Service Usage Admin
- Cloud Run Admin plus Service Account User
- Pub/Sub Admin
- BigQuery Admin for the selected project/dataset lifecycle
- Artifact Registry Administrator
- Secret Manager Admin
- Monitoring Editor
- Service Account Admin and Project IAM Admin for runtime identity bindings
- Storage Object Admin only on the environment state-bucket prefix
- Billing Account Costs Manager only when this root manages a budget

These are deployment permissions, not runtime permissions. Review and reduce them with a custom provisioning role after the first successful plan.

Configure these **GitHub Environment variables** (not repository secrets containing keys):

| Variable | Value |
|---|---|
| `GCP_PROJECT_ID` | approved project ID |
| `GCP_REGION` | `europe-west1` or approved EU region |
| `GCP_STATE_BUCKET` | bootstrap state bucket |
| `GCP_WORKLOAD_IDENTITY_PROVIDER` | full provider resource name |
| `GCP_DEPLOY_SERVICE_ACCOUNT` | deployment service-account email |
| `GCP_CORS_ALLOWED_ORIGINS_JSON` | JSON/HCL list such as `["https://app.example.com"]` |
| `GCP_NOTIFICATION_CHANNEL_IDS_JSON` | list of Monitoring channel IDs |
| `GCP_BILLING_ACCOUNT_ID` | optional billing account ID |

The workflow requests only `contents: read` and `id-token: write`, authenticates through WIF, pushes one `linux/amd64` image, and passes the resulting `@sha256:` reference to OpenTofu. When a billing account is configured, the budget amount uses that account's actual currency rather than assuming EUR/USD. The Google provider attributes API quota to `project_id`, which is required for user-ADC bootstrap of billing APIs.

## Plan and apply

Run `.github/workflows/gcp-deploy.yml` manually:

1. Select `proof` or `production`.
2. Keep `apply=false` to build and review the complete plan.
3. Re-run with `apply=true` only after environment approval.
4. Record the image digest and OpenTofu state generation, never token/key content.

Local equivalent after pushing an immutable image:

```bash
tofu -chdir=infra/gcp fmt -check -recursive
tofu -chdir=infra/gcp validate
tofu -chdir=infra/gcp plan \
  -var-file="${ENVIRONMENT}.tfvars" \
  -var="image=${REGION}-docker.pkg.dev/${PROJECT_ID}/straumheim-${ENVIRONMENT}/straumheim@sha256:<digest>" \
  -out=deploy.tfplan
tofu -chdir=infra/gcp apply deploy.tfplan
```

## IAM result

- collector: config-secret accessor and publisher on one events topic; its route-invoker IAM check is explicitly disabled because tracker traffic is public and organization domain-restricted-sharing policy rejects `allUsers` bindings;
- writer: config-secret accessor and BigQuery Data Editor on one dataset;
- push identity: invoker on the private writer only;
- Pub/Sub service agent: token creation for the push identity plus dead-letter forwarding;
- writer has no `allUsers` binding and retains the IAM invoker check;
- no runtime service account has a downloadable key.

Cloud Run IAM authenticates the push identity before writer application handling. `INGRESS_TRAFFIC_ALL` is required for Pub/Sub's hosted push endpoint; absence of a public writer binding keeps it private. Disabling only the collector's invoker check is Cloud Run's documented public-service mechanism when domain-restricted sharing prevents granting `allUsers`.

## Rollback

Cloud Run retains revisions. During an incident, route traffic to a known digest/revision:

```bash
gcloud run revisions list --service="straumheim-${ENVIRONMENT}-collector" --region="$REGION"
gcloud run services update-traffic "straumheim-${ENVIRONMENT}-collector" \
  --region="$REGION" --to-revisions="<known-good-revision>=100"
```

Repeat for the writer when needed. Update the OpenTofu image variable to the same known-good digest before the next apply so declarative state matches emergency routing.

## Teardown

Proof only:

```bash
tofu -chdir=infra/gcp plan -destroy -var-file=proof.tfvars -out=destroy.tfplan
tofu -chdir=infra/gcp apply destroy.tfplan
```

Verify Cloud Run services, Pub/Sub subscriptions/topics, proof dataset/table, secrets, runtime identities, Artifact Registry images, alert policies, and state objects. Do not set `delete_proof_data_on_destroy=true` for production or destroy production without explicit data-owner approval.
