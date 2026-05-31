resource "google_service_account" "relay" {
  account_id   = "burrow-relay"
  display_name = "Burrow Relay"

  depends_on = [google_project_service.apis["iam.googleapis.com"]]
}

resource "google_compute_instance" "relay" {
  name         = "burrow-relay"
  machine_type = var.relay_machine_type
  zone         = var.zone
  tags         = ["burrow-relay"]

  # Container-Optimized OS has Docker pre-installed and is managed by Google.
  boot_disk {
    initialize_params {
      image = "cos-cloud/cos-stable"
      size  = 10
      type  = "pd-standard"
    }
  }

  network_interface {
    network = "default"
    access_config {
      nat_ip = google_compute_address.relay.address
    }
  }

  service_account {
    email  = google_service_account.relay.email
    scopes = ["cloud-platform"]
  }

  # gce-container-declaration tells COS which container to run on boot.
  metadata = {
    "gce-container-declaration" = yamlencode({
      spec = {
        containers = [{
          image = "${var.region}-docker.pkg.dev/${var.project_id}/burrow/relay:latest"
          env = [
            { name = "LOG_LEVEL", value = var.log_level },
            { name = "TUNNEL_BIND", value = ":8082" },
            { name = "TUNNEL_PUBLIC_URL", value = "http://${google_compute_address.relay.address}:8082" },
          ]
          stdin = false
          tty   = false
        }]
        restartPolicy = "Always"
      }
    })
    "google-logging-enabled" = "true"
  }

  depends_on = [
    google_project_service.apis["compute.googleapis.com"],
    google_artifact_registry_repository_iam_member.relay_pull,
  ]
}
