variable "project_id" {
  description = "Google Cloud project ID"
  type        = string
}

variable "region" {
  description = "Google Cloud region for Cloud Run and Artifact Registry"
  type        = string
  default     = "us-central1"
}

variable "zone" {
  description = "Google Cloud zone for the relay Compute Engine instance"
  type        = string
  default     = "us-central1-a"
}

variable "relay_machine_type" {
  description = "Compute Engine machine type for the relay server"
  type        = string
  default     = "e2-micro"
}

variable "log_level" {
  description = "Log level for exchange and relay servers (debug|info|warn|error)"
  type        = string
  default     = "info"
}

variable "github_repo" {
  description = "GitHub repository allowed to push images, in owner/repo format (e.g. maxcraig112/burrow)"
  type        = string
  default     = "maxcraig112/burrow"
}
