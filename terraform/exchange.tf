resource "google_service_account" "exchange" {
  account_id   = "burrow-exchange"
  display_name = "Burrow Exchange"

  depends_on = [google_project_service.apis["iam.googleapis.com"]]
}

resource "google_cloud_run_v2_service" "exchange" {
  name     = "burrow-exchange"
  location = var.region

  template {
    service_account = google_service_account.exchange.email

    # Must exceed the 5-minute session TTL so WebSocket connections
    # are not killed before the sender/receiver pair completes.
    timeout = "360s"

    containers {
      # Placeholder so the initial `terraform apply` succeeds before any image
      # has been pushed. The GitHub Actions workflow owns subsequent deploys.
      image = "us-docker.pkg.dev/cloudrun/container/hello"

      env {
        name  = "LOG_LEVEL"
        value = var.log_level
      }

      ports {
        container_port = 8080
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 10
    }
  }

  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_artifact_registry_repository_iam_member.exchange_pull,
  ]

  lifecycle {
    # Image updates are managed by the GitHub Actions deploy step, not Terraform.
    ignore_changes = [template[0].containers[0].image]
  }
}

# Allow unauthenticated public access.
resource "google_cloud_run_v2_service_iam_member" "exchange_public" {
  project  = google_cloud_run_v2_service.exchange.project
  location = google_cloud_run_v2_service.exchange.location
  name     = google_cloud_run_v2_service.exchange.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
