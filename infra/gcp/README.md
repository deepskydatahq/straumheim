# GCP-native Straumheim infrastructure

This OpenTofu root provisions the optional M012 profile:

```text
public Cloud Run collector -> Pub/Sub -> private Cloud Run writer -> BigQuery
                                      -> dead-letter topic/subscription
```

It also creates runtime identities, config secrets, Artifact Registry, Cloud Monitoring policies, and an optional budget. Production can enable a global HTTPS uptime check and five-minute Cloud Scheduler webhook canary for the soak. No service-account key is created.

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
| `GCP_CORS_ALLOWED_ORIGINS_JSON` | JSON/HCL list such as `["https://app.example.com"]`; wildcard only with owner approval |
| `GCP_COLLECTOR_DOMAIN` | verified production domain, empty for initial proof |
| `GCP_DATASET_ID` | environment-owned BigQuery dataset ID |
| `GCP_ENABLE_PROPEL_HARNESS_SOURCE` | `true` only for an approved source proof/deployment |
| `GCP_PROPEL_HARNESS_SOURCE_KEY_ID` | approved non-secret source key ID |
| `GCP_PROPEL_HARNESS_SOURCE_SECRET_ID` | existing environment-specific Secret Manager secret resource ID; never its value |
| `GCP_PROPEL_HARNESS_SOURCE_SECRET_VERSION` | exact enabled numeric secret version; never `latest` |
| `GCP_NOTIFICATION_CHANNEL_IDS_JSON` | list of Monitoring channel IDs |
| `GCP_BILLING_ACCOUNT_ID` | optional billing account ID |
| `GCP_MONTHLY_BUDGET_AMOUNT` | amount in the billing account's native currency |

The workflow requests `contents: read`, `actions: read`, and `id-token: write`, authenticates through WIF, and separates planning from applying. A plan run pushes one `linux/amd64` image and binds its `@sha256:` digest into an immutable OpenTofu plan artifact. When a billing account is configured, the budget amount uses that account's actual currency rather than assuming EUR/USD. The Google provider attributes API quota to `project_id`, which is required for user-ADC bootstrap of billing APIs.

## Plan and apply

Run `.github/workflows/gcp-deploy.yml` manually:

1. Dispatch the exact commit with the environment and `operation=plan`. This publishes one immutable image and uploads a three-day plan artifact.
2. Review the commit, environment, image digest, OpenTofu diff, state identity, workflow run ID, and reported plan SHA-256.
3. Dispatch the same exact commit with `operation=apply`, the producing `plan_run_id`, and exact `plan_sha256` only after approval.
4. The apply job downloads that named artifact, verifies all bound identities and bytes, and applies it without rebuilding or replanning.
5. Record the resulting state generation, never token/key content.

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

## Optional authenticated Propel Harness source

The source is disabled by default. A reviewed proof/production plan sets `enable_propel_harness_source=true`, a non-secret `propel_harness_source_key_id`, `propel_harness_source_secret_id` referencing an existing Secret Manager secret, and an exact numeric `propel_harness_source_secret_version`. `latest` is refused so secret bytes and the deployed revision cannot drift independently. Provision and rotate the secret value outside OpenTofu state. The collector alone receives `secretAccessor`; the writer and other sources do not receive the value. Review the generated collector config to confirm that source, vendor, schema, payload version, path, and five-minute clock bound are exact before applying.

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
