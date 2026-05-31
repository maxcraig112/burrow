terraform {
  required_version = ">= 1.5"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }

  # Uncomment to store state in GCS (recommended for teams):
  # backend "gcs" {
  #   bucket = "YOUR_BUCKET"
  #   prefix = "burrow/terraform"
  # }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# Enable required Google Cloud APIs.
resource "google_project_service" "apis" {
  for_each = toset([
    "run.googleapis.com",
    "compute.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
  ])
  service            = each.key
  disable_on_destroy = false
}
