#!/usr/bin/env bash
# Build and push exchange + relay Docker images to Docker Hub.
# Useful for manual builds outside of CI. Requires `docker login` first.
#
# Usage:
#   ./scripts/docker-build.sh <dockerhub-username>
#
# Example:
#   ./scripts/docker-build.sh maximiliancraig112

set -euo pipefail

USERNAME="${1:?Usage: $0 <dockerhub-username>}"

echo "Building exchange..."
docker build -f deploy/exchange.Dockerfile -t "${USERNAME}/burrow-exchange:latest" .

echo ""
echo "Building relay..."
docker build -f deploy/relay.Dockerfile -t "${USERNAME}/burrow-relay:latest" .

echo ""
echo "Pushing images..."
docker push "${USERNAME}/burrow-exchange:latest"
docker push "${USERNAME}/burrow-relay:latest"

echo ""
echo "Done."
