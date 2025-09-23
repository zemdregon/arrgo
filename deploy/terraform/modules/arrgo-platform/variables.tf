variable "release_name" {
  description = "Helm release name for the arrgo deployment."
  type        = string
  default     = "arrgo"
}

variable "namespace" {
  description = "Kubernetes namespace where arrgo should be installed."
  type        = string
  default     = "arrgo"
}

variable "create_namespace" {
  description = "Whether the module should create the target namespace."
  type        = bool
  default     = true
}

variable "values" {
  description = "Map of Helm values to merge with the chart defaults."
  type        = map(any)
  default     = {}
}

variable "wait" {
  description = "Whether to wait until all resources are ready before Terraform completes."
  type        = bool
  default     = true
}

variable "timeout" {
  description = "Time in seconds to wait for Helm operations to complete."
  type        = number
  default     = 600
}
