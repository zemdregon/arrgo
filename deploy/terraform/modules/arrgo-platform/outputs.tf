output "release_name" {
  description = "Helm release name."
  value       = helm_release.arrgo.name
}

output "namespace" {
  description = "Namespace where the arrgo release is installed."
  value       = helm_release.arrgo.namespace
}
