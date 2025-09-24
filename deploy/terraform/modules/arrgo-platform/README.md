# arrgo Terraform Module

This module installs the arrgo Helm chart and, optionally, creates the target
Kubernetes namespace. It is intended to be composed with infrastructure modules
that provision managed PostgreSQL, Redis, and OpenSearch services while also
rolling out the dashboard UI alongside the master and worker workloads.

## Usage

```hcl
module "arrgo" {
  source = "../../modules/arrgo-platform"

  release_name      = "arrgo"
  namespace         = "arrgo"
  create_namespace  = true
  values = {
    image = {
      repository = "ghcr.io/example-org/arrgo"
      tag        = "v0.1.0"
    }
    config = {
      databaseURL   = var.database_url
      redisAddr     = var.redis_addr
      opensearchURL = var.opensearch_url
      masterURL     = var.master_url
      logLevel      = var.log_level
    }
    master = {
      service = {
        type = "LoadBalancer"
      }
    }
    web = {
      image = {
        repository = "ghcr.io/example-org/arrgo-dashboard"
        tag        = "v0.1.0"
      }
      service = {
        type = "LoadBalancer"
      }
    }
    worker = {
      autoscaling = {
        enabled     = true
        minReplicas = 3
        maxReplicas = 12
      }
    }
  }
}
```

The `values` map is converted to YAML and passed directly to Helm. Sensitive
fields (e.g., connection strings) should be sourced from Terraform variables or
secrets managers rather than hard-coded into configuration files.

## Inputs

| Name | Description | Type | Default |
| ---- | ----------- | ---- | ------- |
| `release_name` | Helm release name. | `string` | `"arrgo"` |
| `namespace` | Namespace used for the deployment. | `string` | `"arrgo"` |
| `create_namespace` | Whether to create the namespace. | `bool` | `true` |
| `values` | Additional Helm values encoded as a map. | `map(any)` | `{}` |
| `wait` | Wait for resources to become ready. | `bool` | `true` |
| `timeout` | Helm operation timeout in seconds. | `number` | `600` |

## Outputs

| Name | Description |
| ---- | ----------- |
| `release_name` | Helm release name. |
| `namespace` | Namespace where the release is installed. |

## Provider Configuration

Configure the Terraform `helm` and `kubernetes` providers in the root module.
For example:

```hcl
provider "kubernetes" {
  host                   = var.cluster_endpoint
  cluster_ca_certificate = base64decode(var.cluster_ca)
  token                  = var.cluster_token
}

provider "helm" {
  kubernetes {
    host                   = var.cluster_endpoint
    cluster_ca_certificate = base64decode(var.cluster_ca)
    token                  = var.cluster_token
  }
}
```
