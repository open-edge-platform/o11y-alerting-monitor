{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "alerting-monitor.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "alerting-monitor.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "alerting-monitor.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Selector labels for management
*/}}
{{- define "alerting-monitor-management.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}-management
app.kubernetes.io/instance: {{ .Release.Name }}-management
{{- end }}

{{/*
Common labels
*/}}
{{- define "alerting-monitor.labels" -}}
helm.sh/chart: {{ include "alerting-monitor.chart" . }}
{{ include "alerting-monitor.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Common labels for management
*/}}
{{- define "alerting-monitor-management.labels" -}}
helm.sh/chart: {{ include "alerting-monitor.chart" . }}
{{ include "alerting-monitor-management.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Ports definitions
*/}}
{{- define "alerting-monitor-management.ports.grpc" -}}
  51001
{{- end -}}
