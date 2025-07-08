variable "gcp_project_id" {
  type = string
}

variable "gcp_region" {
  type    = string
  default = "us-central1"
}

variable "gcp_zone" {
  type    = string
  default = "us-central1-c"
}

variable "tags" {
  type    = map(string)
  default = {}
}

variable "wi_k8s_sa_ns" {
  type        = string
  description = "Namespace of kubernetes service account to be bound to GCP IAM service account (For workload identity)"
}

variable "wi_k8s_sa_name" {
  type        = string
  description = "Name of kubernetes service account to be bound to GCP IAM service account (For workload identity)"
}

variable "wi_k8s_sa_name_direct_access" {
  type        = string
  description = "Name of kubernetes service account to get direct permissions in GCP (For workload identity)"
}

variable "wi_k8s_sa_name_federation" {
  type        = string
  description = "Name of kubernetes service account to be bound to GCP IAM service account (For workload identity federation)"
}

variable "wi_k8s_sa_name_federation_direct_access" {
  type        = string
  description = "Name of kubernetes service account to get direct permissions in GCP (For workload identity federation)"
}

variable "enable_wi" {
  type        = bool
  default     = false
  description = "Enable workload identity on cluster and create a federated identity with service account"
}
