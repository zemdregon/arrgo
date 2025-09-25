{{- define "arrgo.name" -}}
{{- default .Chart.Name .Values.nameOverride -}}
{{- end -}}

{{- define "arrgo.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "arrgo.chart" -}}
{{- printf "%s-%s" .Chart.Name (.Chart.Version | replace "+" "_") -}}
{{- end -}}

{{- define "arrgo.labels" -}}
helm.sh/chart: {{ include "arrgo.chart" . }}
{{ include "arrgo.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
{{- end -}}

{{- define "arrgo.selectorLabels" -}}
app.kubernetes.io/name: {{ include "arrgo.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "arrgo.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (printf "%s-sa" (include "arrgo.fullname" .)) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "arrgo.configSecretName" -}}
{{- if .Values.config.existingSecret -}}
{{- .Values.config.existingSecret -}}
{{- else if .Values.config.secretName -}}
{{- .Values.config.secretName -}}
{{- else -}}
{{- printf "%s-config" (include "arrgo.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "arrgo.master.fullname" -}}
{{ printf "%s-master" (include "arrgo.fullname" .) }}
{{- end -}}

{{- define "arrgo.worker.fullname" -}}
{{ printf "%s-worker" (include "arrgo.fullname" .) }}
{{- end -}}

{{- define "arrgo.master.serviceName" -}}
{{ include "arrgo.master.fullname" . }}
{{- end -}}

{{- define "arrgo.databaseURL" -}}
{{- $cfg := .Values.config -}}
{{- if $cfg.databaseURL -}}
{{- $cfg.databaseURL -}}
{{- else -}}
{{- $db := .Values.database -}}
{{- if $db.host -}}
{{- $user := default "arrgo" $db.user -}}
{{- $name := default "arrgo" $db.name -}}
{{- $port := default 5432 $db.port -}}
{{- $ssl := default "disable" $db.sslMode -}}
{{- if $db.password -}}
{{- printf "postgres://%s:%s@%s:%v/%s?sslmode=%s" $user $db.password $db.host $port $name $ssl -}}
{{- else -}}
{{- printf "postgres://%s@%s:%v/%s?sslmode=%s" $user $db.host $port $name $ssl -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "arrgo.redisAddr" -}}
{{- $cfg := .Values.config -}}
{{- if $cfg.redisAddr -}}
{{- $cfg.redisAddr -}}
{{- else -}}
{{- $redis := .Values.redis -}}
{{- if $redis.host -}}
{{- $port := default 6379 $redis.port -}}
{{- printf "%s:%v" $redis.host $port -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "arrgo.opensearchURL" -}}
{{- $cfg := .Values.config -}}
{{- if $cfg.opensearchURL -}}
{{- $cfg.opensearchURL -}}
{{- else -}}
{{- $os := .Values.opensearch -}}
{{- if $os.host -}}
{{- $protocol := default "http" $os.protocol -}}
{{- $port := default 9200 $os.port -}}
{{- if and $os.username $os.password -}}
{{- printf "%s://%s:%s@%s:%v" $protocol $os.username $os.password $os.host $port -}}
{{- else -}}
{{- printf "%s://%s:%v" $protocol $os.host $port -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "arrgo.masterURL" -}}
{{- if .Values.config.masterURL -}}
{{- .Values.config.masterURL -}}
{{- else -}}
{{- $service := include "arrgo.master.serviceName" . -}}
{{- $port := default 8080 .Values.master.service.port -}}
{{- printf "http://%s:%v" $service $port -}}
{{- end -}}
{{- end -}}
{{- define "arrgo.master.labels" -}}
{{ include "arrgo.labels" . }}
app.kubernetes.io/component: master
{{- end -}}

{{- define "arrgo.master.selectorLabels" -}}
{{ include "arrgo.selectorLabels" . }}
app.kubernetes.io/component: master
{{- end -}}

{{- define "arrgo.worker.labels" -}}
{{ include "arrgo.labels" . }}
app.kubernetes.io/component: worker
{{- end -}}

{{- define "arrgo.worker.selectorLabels" -}}
{{ include "arrgo.selectorLabels" . }}
app.kubernetes.io/component: worker
{{- end -}}

{{- define "arrgo.dashboard.fullname" -}}
{{ printf "%s-dashboard" (include "arrgo.fullname" .) }}
{{- end -}}

{{- define "arrgo.dashboard.serviceName" -}}
{{ include "arrgo.dashboard.fullname" . }}
{{- end -}}

{{- define "arrgo.dashboard.labels" -}}
{{ include "arrgo.labels" . }}
app.kubernetes.io/component: dashboard
{{- end -}}

{{- define "arrgo.dashboard.selectorLabels" -}}
{{ include "arrgo.selectorLabels" . }}
app.kubernetes.io/component: dashboard
{{- end -}}

{{- define "arrgo.observability.prometheus.fullname" -}}
{{ printf "%s-prometheus" (include "arrgo.fullname" .) }}
{{- end -}}

{{- define "arrgo.observability.prometheus.serviceName" -}}
{{ include "arrgo.observability.prometheus.fullname" . }}
{{- end -}}

{{- define "arrgo.observability.prometheus.labels" -}}
{{ include "arrgo.labels" . }}
app.kubernetes.io/component: prometheus
{{- end -}}

{{- define "arrgo.observability.prometheus.selectorLabels" -}}
{{ include "arrgo.selectorLabels" . }}
app.kubernetes.io/component: prometheus
{{- end -}}

{{- define "arrgo.observability.alertmanager.fullname" -}}
{{ printf "%s-alertmanager" (include "arrgo.fullname" .) }}
{{- end -}}

{{- define "arrgo.observability.alertmanager.serviceName" -}}
{{ include "arrgo.observability.alertmanager.fullname" . }}
{{- end -}}

{{- define "arrgo.observability.alertmanager.labels" -}}
{{ include "arrgo.labels" . }}
app.kubernetes.io/component: alertmanager
{{- end -}}

{{- define "arrgo.observability.alertmanager.selectorLabels" -}}
{{ include "arrgo.selectorLabels" . }}
app.kubernetes.io/component: alertmanager
{{- end -}}

{{- define "arrgo.observability.grafana.fullname" -}}
{{ printf "%s-grafana" (include "arrgo.fullname" .) }}
{{- end -}}

{{- define "arrgo.observability.grafana.serviceName" -}}
{{ include "arrgo.observability.grafana.fullname" . }}
{{- end -}}

{{- define "arrgo.observability.grafana.labels" -}}
{{ include "arrgo.labels" . }}
app.kubernetes.io/component: grafana
{{- end -}}

{{- define "arrgo.observability.grafana.selectorLabels" -}}
{{ include "arrgo.selectorLabels" . }}
app.kubernetes.io/component: grafana
{{- end -}}

{{- define "arrgo.observability.otelCollector.fullname" -}}
{{ printf "%s-otel-collector" (include "arrgo.fullname" .) }}
{{- end -}}

{{- define "arrgo.observability.otelCollector.serviceName" -}}
{{ include "arrgo.observability.otelCollector.fullname" . }}
{{- end -}}

{{- define "arrgo.observability.otelCollector.labels" -}}
{{ include "arrgo.labels" . }}
app.kubernetes.io/component: otel-collector
{{- end -}}

{{- define "arrgo.observability.otelCollector.selectorLabels" -}}
{{ include "arrgo.selectorLabels" . }}
app.kubernetes.io/component: otel-collector
{{- end -}}
