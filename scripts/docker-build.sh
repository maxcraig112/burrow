#!/usr/bin/env bash
# Build and push exchange + relay Docker images to Artifact Registry.
# Run this from the repo root before `terraform apply`.
#
# Usage:
#   ./scripts/docker-build.sh <project-id> [region]
#
# Example:
#   ./scripts/docker-build.sh my-gcp-project us-central1

set -euo pipefail

PROJECT_ID="${1:?Usage: $0 <project-id> [region]}"
REGION="${2:-us-central1}"
REGISTRY="${REGION}-docker.pkg.dev/${PROJECT_ID}/burrow"

echo "Configuring Docker for Artifact Registry..."
gcloud auth configure-docker "${REGION}-docker.pkg.dev" --quiet

echo ""
echo "Building exchange..."
docker build -f deploy/exchange.Dockerfile -t "${REGISTRY}/exchange:latest" .

echo ""
echo "Building relay..."
docker build -f deploy/relay.Dockerfile -t "${REGISTRY}/relay:latest" .

echo ""
echo "Pushing images..."
docker push "${REGISTRY}/exchange:latest"
docker push "${REGISTRY}/relay:latest"

echo ""
echo "Done. You can now run: terraform -chdir=terraform apply"
