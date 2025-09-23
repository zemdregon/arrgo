# arrgo Helm Chart

This chart deploys the arrgo control plane (`master`) and scalable worker pool on
a Kubernetes cluster. It is designed to plug into production-grade backing
services such as managed PostgreSQL, Redis, and OpenSearch deployments while
providing sane defaults for security contexts, autoscaling, and observability
integration.

## Prerequisites

- Kubernetes 1.25+
- Helm 3.9+
- External PostgreSQL, Redis, and OpenSearch endpoints or values that allow the
  chart to generate connection strings.

## Installing the Chart

```bash
helm repo add arrgo-local ./deploy/helm
helm install arrgo arrgo-local/arrgo \
  --namespace arrgo \
  --create-namespace \
  --set config.databaseURL="postgres://arrgo:strong-password@postgresql:5432/arrgo?sslmode=require" \
  --set config.redisAddr="redis-master:6379" \
  --set config.opensearchURL="https://opensearch.example.com:9200" \
  --set config.masterURL="https://arrgo.example.com"
```

The chart automatically provisions a secret named `<release>-arrgo-config`
(controllable through `config.secretName`) when `config.createSecret` is true.
Provide explicit connection strings when referencing managed services or disable
secret creation and point the chart at an existing secret.

## Key Values

| Value | Description | Default |
| ----- | ----------- | ------- |
| `image.repository` | arrgo container image repository. | `ghcr.io/example-org/arrgo` |
| `config.createSecret` | Generate a Kubernetes secret with connection settings. | `true` |
| `config.existingSecret` | Reference an existing secret instead of generating one. | `""` |
| `config.logLevel` | Log level applied to master and worker pods. | `info` |
| `database.*` | Connection hints used to build the PostgreSQL URL when `config.databaseURL` is empty. | See `values.yaml` |
| `master.replicaCount` | Number of master pods. | `2` |
| `master.service.type` | Kubernetes service type for the master API. | `ClusterIP` |
| `master.ingress.enabled` | Create an Ingress resource for the master API. | `false` |
| `worker.replicaCount` | Baseline worker replica count when autoscaling is disabled. | `3` |
| `worker.autoscaling.enabled` | Enable HorizontalPodAutoscaler for workers. | `true` |

Refer to [`values.yaml`](./values.yaml) for the complete set of tunable options.

## Secrets and Connection Management

For production environments it is recommended to provision the connection secret
independently and set `config.createSecret=false` alongside
`config.existingSecret=<secret-name>`. The secret must define the following
keys:

- `ARRGO_LOG_LEVEL`
- `ARRGO_DATABASE_URL`
- `ARRGO_REDIS_ADDR`
- `ARRGO_OPENSEARCH_URL`
- `ARRGO_MASTER_URL`

The Terraform module in [`deploy/terraform`](../terraform) demonstrates how to
codify secret creation and Helm installation in a single workflow.

## Uninstalling the Chart

```bash
helm uninstall arrgo --namespace arrgo
```

Persistent data for Postgres, Redis, or OpenSearch is left intact when those
services are managed outside the chart.
