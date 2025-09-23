variable "release_name" {
  description = "Helm release name for arrgo."
  type        = string
  default     = "arrgo"
}

variable "namespace" {
  description = "Namespace used for arrgo resources."
  type        = string
  default     = "arrgo"
}

variable "config_secret_name" {
  description = "Name of the Kubernetes secret that stores arrgo connection details."
  type        = string
  default     = "arrgo-production-config"
}

variable "database_url" {
  description = "PostgreSQL connection string for arrgo."
  type        = string
}

variable "redis_addr" {
  description = "Redis endpoint in host:port format."
  type        = string
}

variable "opensearch_url" {
  description = "OpenSearch endpoint URL."
  type        = string
}

variable "master_url" {
  description = "Externally reachable URL for the arrgo master API."
  type        = string
}

variable "log_level" {
  description = "Log level for master and worker pods."
  type        = string
  default     = "info"
}

variable "image_repository" {
  description = "Container image repository for arrgo."
  type        = string
  default     = "ghcr.io/example-org/arrgo"
}

variable "image_tag" {
  description = "Container image tag to deploy."
  type        = string
  default     = "main"
}

variable "master_service_type" {
  description = "Service type for the arrgo master Kubernetes Service."
  type        = string
  default     = "LoadBalancer"
}

variable "worker_min_replicas" {
  description = "Minimum number of worker replicas."
  type        = number
  default     = 3
}

variable "worker_max_replicas" {
  description = "Maximum number of worker replicas."
  type        = number
  default     = 12
}

variable "worker_cpu_target" {
  description = "Target CPU utilization percentage for worker autoscaling."
  type        = number
  default     = 70
}
