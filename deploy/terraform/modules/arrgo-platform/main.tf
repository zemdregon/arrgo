locals {
  chart_path = "${path.module}/../../helm/arrgo"
}

resource "kubernetes_namespace" "this" {
  count = var.create_namespace ? 1 : 0

  metadata {
    name = var.namespace
  }
}

resource "helm_release" "arrgo" {
  name       = var.release_name
  namespace  = var.create_namespace ? kubernetes_namespace.this[0].metadata[0].name : var.namespace
  chart      = local.chart_path
  atomic     = true
  cleanup_on_fail = true
  wait       = var.wait
  timeout    = var.timeout

  values = length(var.values) == 0 ? [] : [yamlencode(var.values)]
}
