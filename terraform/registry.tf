resource "google_artifact_registry_repository" "burrow" {
  location      = var.region
  repository_id = "burrow"
  format        = "DOCKER"

  depends_on = [google_project_service.apis["artifactregistry.googleapis.com"]]
}

# Grant the exchange service account read access to pull images.
resource "google_artifact_registry_repository_iam_member" "exchange_pull" {
  project    = var.project_id
  location   = var.region
  repository = google_artifact_registry_repository.burrow.name
  role       = "roles/artifactregistry.reader"
  member     = "serviceAccount:${google_service_account.exchange.email}"
}

# Grant the relay service account read access to pull images.
resource "google_artifact_registry_repository_iam_member" "relay_pull" {
  project    = var.project_id
  location   = var.region
  repository = google_artifact_registry_repository.burrow.name
  role       = "roles/artifactregistry.reader"
  member     = "serviceAccount:${google_service_account.relay.email}"
}
