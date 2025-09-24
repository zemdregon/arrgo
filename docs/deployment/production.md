# Production Deployment Guide

This guide describes how to deploy arrgo into a production Kubernetes cluster
using the new Helm chart and Terraform module introduced in this repository.

## Overview

Production deployments rely on the following building blocks:

- **arrgo Helm chart** – Deploys the control-plane (`master`), worker pods, and
  dashboard UI, configures Kubernetes services/ingress, and wires connection
  details via secrets.
- **Terraform module** – Automates Helm installation, namespace creation, and
  secret management.
- **Managed data services** – PostgreSQL, Redis, and OpenSearch instances
  provisioned outside of the chart (e.g., via cloud-managed offerings or
  separate Terraform modules).

## Helm Deployment

The chart is located at [`deploy/helm/arrgo`](../../deploy/helm/arrgo).
Populate connection settings for PostgreSQL, Redis, and OpenSearch either by
providing explicit URLs or by letting the chart build URLs from host/credential
values.

```bash
helm install arrgo ./deploy/helm/arrgo \
  --namespace arrgo \
  --create-namespace \
  --set config.databaseURL="postgres://arrgo:strong-password@postgresql:5432/arrgo?sslmode=require" \
  --set config.redisAddr="redis-master:6379" \
  --set config.opensearchURL="https://opensearch.example.com:9200" \
  --set config.masterURL="https://arrgo.example.com"
```

To keep credentials out of the command line, create a Kubernetes secret ahead of
install time and set `config.createSecret=false` with
`config.existingSecret=<secret-name>`.

## Terraform Automation

The module in [`deploy/terraform/modules/arrgo-platform`](../../deploy/terraform/modules/arrgo-platform)
wraps Helm installation and (optionally) namespace creation. Combine it with
resources that manage the connection secret to achieve a repeatable deployment.
An end-to-end example is included at
[`deploy/terraform/environments/production`](../../deploy/terraform/environments/production).

High-level workflow:

1. Configure the `kubernetes` and `helm` providers (supports exec, token, or
   kubeconfig authentication flows).
2. Populate `terraform.tfvars` with the connection strings for managed data
   stores.
3. Run `terraform init && terraform apply` to create the namespace, secret, and
   Helm release.

The example environment provisions a secret named `arrgo-production-config` with
these keys:

- `ARRGO_DATABASE_URL`
- `ARRGO_REDIS_ADDR`
- `ARRGO_OPENSEARCH_URL`
- `ARRGO_MASTER_URL`
- `ARRGO_LOG_LEVEL`

The `ARRGO_MASTER_URL` value is also injected into the dashboard pods so
browser-based API requests resolve to the externally reachable master endpoint
(for example the master ingress host).

## Additional Considerations

- Adjust `worker.autoscaling` values to align with cluster capacity and desired
  throughput.
- Use Kubernetes ingress controllers or service meshes to expose the master API
  securely. The chart supports ingress annotations for TLS termination and HTTP
  routing.
- Integrate with a secret manager (e.g., HashiCorp Vault, AWS Secrets Manager)
  for automated credential rotation. Feed the rendered values into the
  Kubernetes secret resource used by the chart.
- Update monitoring dashboards and alerts to track master/worker availability
  and queue depth once arrgo emits metrics.
