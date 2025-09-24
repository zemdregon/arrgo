# Production Environment Example

This configuration demonstrates how to deploy arrgo into a production namespace
using the Terraform module in [`../../modules/arrgo-platform`](../../modules/arrgo-platform).
It provisions the namespace, creates the secret that stores connection strings,
and installs the Helm chart with worker autoscaling enabled.

## Usage

1. Configure the Terraform `helm` and `kubernetes` providers in `providers.tf` or
   through your preferred authentication method.
2. Populate a `terraform.tfvars` file (see the example below) with connection
   information for managed PostgreSQL, Redis, and OpenSearch instances.
3. Initialize and apply the configuration:

   ```bash
   terraform init
   terraform apply
   ```

## Example `terraform.tfvars`

```hcl
release_name        = "arrgo"
namespace           = "media-automation"
config_secret_name  = "arrgo-production-config"
database_url        = "postgres://arrgo:REDACTED@prod-postgres:5432/arrgo?sslmode=require"
redis_addr          = "prod-redis:6379"
opensearch_url      = "https://search.example.com"
master_url          = "https://arrgo.example.com"
log_level           = "info"
image_repository    = "ghcr.io/example-org/arrgo"
image_tag           = "v0.1.0"
web_image_repository = "ghcr.io/example-org/arrgo-dashboard"
web_image_tag        = "v0.1.0"
web_service_type     = "LoadBalancer"
master_service_type = "LoadBalancer"
worker_min_replicas = 3
worker_max_replicas = 15
worker_cpu_target   = 65
```

## Notes

- Secrets are managed as Terraform inputs and rendered into a Kubernetes secret.
  For stronger secrecy guarantees integrate with Vault, AWS Secrets Manager, or a
  similar tool and template the data into the secret resource.
- Adjust the Helm values map in `locals.helm_values` if you need to override
  additional chart settings (ingress, tolerations, resource requests, etc.).
