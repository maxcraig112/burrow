resource "google_compute_address" "relay" {
  name   = "burrow-relay-ip"
  region = var.region

  depends_on = [google_project_service.apis["compute.googleapis.com"]]
}

# Open the relay's two ports to the world.
# Port 9090 — raw TCP file transfer relay
# Port 8082 — HTTP web-upload tunnel proxy
resource "google_compute_firewall" "relay" {
  name    = "burrow-relay"
  network = "default"

  allow {
    protocol = "tcp"
    ports    = ["9090", "8082"]
  }

  source_ranges = ["0.0.0.0/0"]
  target_tags   = ["burrow-relay"]

  depends_on = [google_project_service.apis["compute.googleapis.com"]]
}
