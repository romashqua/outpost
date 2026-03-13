{{/*
Expand the name of the chart.
*/}}
{{- define "outpost.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "outpost.fullname" -}}
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
Chart label helper.
*/}}
{{- define "outpost.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "outpost.labels" -}}
helm.sh/chart: {{ include "outpost.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/part-of: outpost
{{- end }}

{{/*
Selector labels for a given component.
Call with: include "outpost.selectorLabels" (dict "root" . "component" "core")
*/}}
{{- define "outpost.selectorLabels" -}}
app.kubernetes.io/name: {{ include "outpost.name" .root }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Image reference for a component.
Call with: include "outpost.image" (dict "image" .Values.core.image "appVersion" .Chart.AppVersion)
*/}}
{{- define "outpost.image" -}}
{{- $tag := default .appVersion .image.tag -}}
{{- printf "%s:%s" .image.repository $tag }}
{{- end }}

{{/*
Database host — built-in or external.
*/}}
{{- define "outpost.databaseHost" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "%s-postgresql" (include "outpost.fullname" .) }}
{{- else }}
{{- .Values.externalDatabase.host }}
{{- end }}
{{- end }}

{{/*
Database port.
*/}}
{{- define "outpost.databasePort" -}}
{{- if .Values.postgresql.enabled }}
{{- "5432" }}
{{- else }}
{{- .Values.externalDatabase.port | toString }}
{{- end }}
{{- end }}

{{/*
Database name.
*/}}
{{- define "outpost.databaseName" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.database }}
{{- else }}
{{- .Values.externalDatabase.database }}
{{- end }}
{{- end }}

{{/*
Database user.
*/}}
{{- define "outpost.databaseUser" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.username }}
{{- else }}
{{- .Values.externalDatabase.username }}
{{- end }}
{{- end }}

{{/*
Redis host — built-in or external.
*/}}
{{- define "outpost.redisHost" -}}
{{- if .Values.redis.enabled }}
{{- printf "%s-redis" (include "outpost.fullname" .) }}
{{- else }}
{{- .Values.externalRedis.host }}
{{- end }}
{{- end }}

{{/*
Redis port.
*/}}
{{- define "outpost.redisPort" -}}
{{- if .Values.redis.enabled }}
{{- "6379" }}
{{- else }}
{{- .Values.externalRedis.port | toString }}
{{- end }}
{{- end }}

{{/*
Secret name.
*/}}
{{- define "outpost.secretName" -}}
{{- printf "%s-secret" (include "outpost.fullname" .) }}
{{- end }}

{{/*
ConfigMap name.
*/}}
{{- define "outpost.configmapName" -}}
{{- printf "%s-config" (include "outpost.fullname" .) }}
{{- end }}

{{/*
Image pull secrets.
*/}}
{{- define "outpost.imagePullSecrets" -}}
{{- range .Values.imagePullSecrets }}
- name: {{ . }}
{{- end }}
{{- end }}
