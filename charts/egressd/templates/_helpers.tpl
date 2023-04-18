{{/*
Expand the name of the chart.
*/}}
{{- define "egressd.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "egressd.fullname" -}}
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
Create chart name and version as used by the chart label.
*/}}
{{- define "egressd.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "egressd.labels" -}}
helm.sh/chart: {{ include "egressd.chart" . }}
{{ include "egressd.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "egressd.selectorLabels" -}}
app.kubernetes.io/name: {{ include "egressd.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account for collector
*/}}
{{- define "egressd.collector.serviceAccountName" -}}
{{- if .Values.collector.serviceAccount.create }}
{{- default (include "egressd.fullname" .) .Values.collector.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.collector.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the service account for exporter
*/}}
{{- define "egressd.exporter.serviceAccountName" -}}
{{- if .Values.exporter.serviceAccount.create }}
{{- default (include "egressd.exporter.fullname" .) .Values.exporter.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.exporter.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "egressd.exporter.fullname" -}}
{{ include "egressd.fullname" . }}-exporter
{{- end }}

{{/*
Common exporter labels
*/}}
{{- define "egressd.exporter.labels" -}}
helm.sh/chart: {{ include "egressd.chart" . }}
{{ include "egressd.exporter.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "egressd.exporter.selectorLabels" -}}
app.kubernetes.io/name: {{ include "egressd.name" . }}-exporter
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "egressd.export.http.addr" -}}
{{- if .Values.export.http.addr }}
{{- .Values.export.http.addr}}
{{- else }}
{{- printf "http://%s:80" (include "egressd.exporter.fullname" .) }}
{{- end }}
{{- end }}

{{- define "egressd.export.file.addr" -}}
{{- if .Values.export.http.addr }}
{{- .Values.export.http.addr}}
{{- else }}
{{- printf "%s:6000" (include "egressd.exporter.fullname" .) }}
{{- end }}
{{- end }}
