# Service account used by GitHub Actions to push Docker images.
resource "google_service_account" "github_actions" {
  account_id   = "burrow-github-actions"
  display_name = "Burrow GitHub Actions"

  depends_on = [google_project_service.apis["iam.googleapis.com"]]
}

# Grant the service account write access to push images to Artifact Registry.
resource "google_artifact_registry_repository_iam_member" "github_push" {
  project    = var.project_id
  location   = var.region
  repository = google_artifact_registry_repository.burrow.name
  role       = "roles/artifactregistry.writer"
  member     = "serviceAccount:${google_service_account.github_actions.email}"
}

# Grant the service account permission to deploy new revisions to Cloud Run.
resource "google_project_iam_member" "github_run_developer" {
  project = var.project_id
  role    = "roles/run.developer"
  member  = "serviceAccount:${google_service_account.github_actions.email}"

  depends_on = [google_project_service.apis["run.googleapis.com"]]
}

# Workload Identity Pool — the trust boundary for external identities.
resource "google_iam_workload_identity_pool" "github" {
  workload_identity_pool_id = "burrow-github"
  display_name              = "Burrow GitHub Actions"

  depends_on = [google_project_service.apis["iamcredentials.googleapis.com"]]
}

# OIDC provider inside the pool — maps GitHub Actions JWT claims.
resource "google_iam_workload_identity_pool_provider" "github" {
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-actions"
  display_name                       = "GitHub Actions"

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.repository" = "assertion.repository"
    "attribute.ref"        = "assertion.ref"
    "attribute.actor"      = "assertion.actor"
  }

  # Only tokens from this specific repository are accepted.
  attribute_condition = "attribute.repository == \"${var.github_repo}\""
}

# Allow the pool to impersonate the GitHub Actions service account,
# but only for tokens that originated from the correct repository.
resource "google_service_account_iam_member" "github_wif_binding" {
  service_account_id = google_service_account.github_actions.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository/${var.github_repo}"
}
