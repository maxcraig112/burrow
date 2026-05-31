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
- **Docker images** — hosted on Docker Hub and pulled by both GCP services at deploy/boot time.

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform) >= 1.5
- [Google Cloud SDK](https://cloud.google.com/sdk) (`gcloud`)
- A GCP project with billing enabled

## First-time setup

```bash
# 1. Fill in your GCP project ID
cp terraform/terraform.tfvars.example terraform/terraform.tfvars
$EDITOR terraform/terraform.tfvars

# 2. Push Docker images to Docker Hub first (CI will handle this on every push,
#    but for the first deploy you can push manually)
./scripts/docker-build.sh YOUR_DOCKERHUB_USERNAME

# 3. Deploy infrastructure
cd terraform
terraform init
terraform apply
```

After apply, get the addresses for the CLI:

```bash
terraform output burrow_env
```

## GitHub Actions CI/CD

The workflow in `.github/workflows/docker-build.yml` runs on every push to `main` and:
1. Builds and pushes images to Docker Hub (amd64 + arm64)
2. Deploys the exchange service to Cloud Run
3. Resets the relay VM so it boots with the new image

It uses Workload Identity Federation (keyless auth). After `terraform apply`, run:

```bash
terraform output github_workload_identity_provider
terraform output github_actions_service_account
```

Add these as GitHub repository secrets:

| Secret | Value |
| --- | --- |
| `GCP_PROJECT_ID` | your GCP project ID |
| `GCP_WORKLOAD_IDENTITY_PROVIDER` | output from above |
| `GCP_SERVICE_ACCOUNT` | output from above |
| `DOCKERHUB_USERNAME` | your Docker Hub username |
| `DOCKERHUB_TOKEN` | Docker Hub access token |
