locals {
  helm_values = {
    image = {
      repository = var.image_repository
      tag        = var.image_tag
    }
    config = {
      createSecret   = false
      existingSecret = var.config_secret_name
    }
    master = {
      service = {
        type = var.master_service_type
      }
    }
    worker = {
      autoscaling = {
        enabled                             = true
        minReplicas                         = var.worker_min_replicas
        maxReplicas                         = var.worker_max_replicas
        targetCPUUtilizationPercentage      = var.worker_cpu_target
      }
    }
  }
}

resource "kubernetes_namespace" "arrgo" {
  metadata {
    name = var.namespace
  }
}

resource "kubernetes_secret" "arrgo_config" {
  metadata {
    name      = var.config_secret_name
    namespace = kubernetes_namespace.arrgo.metadata[0].name
  }

  data = {
    ARRGO_DATABASE_URL   = var.database_url
    ARRGO_REDIS_ADDR     = var.redis_addr
    ARRGO_OPENSEARCH_URL = var.opensearch_url
    ARRGO_MASTER_URL     = var.master_url
    ARRGO_LOG_LEVEL      = var.log_level
  }

  depends_on = [kubernetes_namespace.arrgo]
}

module "arrgo" {
  source = "../../modules/arrgo-platform"

  release_name     = var.release_name
  namespace        = var.namespace
  create_namespace = false
  values           = local.helm_values

  depends_on = [kubernetes_secret.arrgo_config]
}
