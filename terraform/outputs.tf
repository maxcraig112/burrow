output "exchange_url" {
  description = "Cloud Run exchange server URL (use as EXCHANGE_ADDR)"
  value       = google_cloud_run_v2_service.exchange.uri
}

output "relay_ip" {
  description = "Relay server static IP address"
  value       = google_compute_address.relay.address
}

output "relay_addr" {
  description = "Relay TCP address (use as RELAY_ADDR)"
  value       = "${google_compute_address.relay.address}:9090"
}

output "tunnel_public_url" {
  description = "Web-upload tunnel base URL"
  value       = "http://${google_compute_address.relay.address}:8082"
}

output "wormhole_env" {
  description = "Ready-to-paste .env snippet for the wormhole CLI"
  value       = <<-EOF
    EXCHANGE_ADDR=${google_cloud_run_v2_service.exchange.uri}
    RELAY_ADDR=${google_compute_address.relay.address}:9090
  EOF
}

output "registry" {
  description = "Artifact Registry path prefix (used in build-push script)"
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/burrow"
}

output "github_workload_identity_provider" {
  description = "Value for the GCP_WORKLOAD_IDENTITY_PROVIDER GitHub Actions secret"
  value       = google_iam_workload_identity_pool_provider.github.name
}

output "github_actions_service_account" {
  description = "Value for the GCP_SERVICE_ACCOUNT GitHub Actions secret"
  value       = google_service_account.github_actions.email
}
