# Deployment

## Architecture

```text
┌──────────┐   WebSocket/PAKE   ┌──────────────┐
│  Sender  │ ─────────────────► │   Exchange   │
└──────────┘   HTTP/PAKE        │   Server     │
┌──────────┐ ◄───────────────── └──────────────┘
│ Receiver │
└──────────┘
      │
      │  TCP (AES-256-GCM chunks)
      ▼
┌──────────────┐
│    Relay     │  ── TCP :9090  (file transfer)
│    Server    │  ── HTTP :8082 (web-upload tunnel)
└──────────────┘
```

- **Exchange server** — runs on Cloud Run (HTTP + WebSocket, scales to zero). Coordinates sessions via X25519 ECDH key exchange; never sees file data.
- **Relay server** — runs on a Compute Engine VM (needs a raw TCP port). Splices encrypted bytes between peers; never decrypts anything. Also hosts the HTTP tunnel for `receive-web`.

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform) >= 1.5
- [Google Cloud SDK](https://cloud.google.com/sdk) (`gcloud`)
- Docker
- A GCP project with billing enabled

## First-time setup

```bash
# 1. Fill in your GCP project ID
cp terraform/terraform.tfvars.example terraform/terraform.tfvars
$EDITOR terraform/terraform.tfvars

# 2. Bootstrap the registry and static IP
cd terraform
terraform init
terraform apply \
  -target=google_artifact_registry_repository.burrow \
  -target=google_compute_address.relay

# 3. Build and push Docker images
cd ..
./scripts/docker-build.sh YOUR_PROJECT_ID

# 4. Deploy everything
cd terraform && terraform apply
```

After apply, get the `.env` values for the CLI:

```bash
terraform output burrow_env
```

## GitHub Actions CI/CD

The workflow in `.github/workflows/docker-build.yml` builds and pushes images on every push to `main`, then deploys the exchange service to Cloud Run automatically.

It uses Workload Identity Federation (keyless auth). The required GCP resources are managed by Terraform — after `terraform apply`, run:

```bash
terraform output github_workload_identity_provider
terraform output github_actions_service_account
```

Add these as GitHub repository secrets along with `GCP_PROJECT_ID`:

| Secret | Value |
| --- | --- |
| `GCP_PROJECT_ID` | your GCP project ID |
| `GCP_WORKLOAD_IDENTITY_PROVIDER` | output from above |
| `GCP_SERVICE_ACCOUNT` | output from above |
