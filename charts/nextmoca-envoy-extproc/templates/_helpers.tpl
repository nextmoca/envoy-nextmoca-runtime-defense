{{- define "nextmoca-envoy-extproc.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "nextmoca-envoy-extproc.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name (include "nextmoca-envoy-extproc.name" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{- define "nextmoca-envoy-extproc.labels" -}}
app.kubernetes.io/name: {{ include "nextmoca-envoy-extproc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | quote }}
{{- end }}

{{- define "nextmoca-envoy-extproc.selectorLabels" -}}
app.kubernetes.io/name: {{ include "nextmoca-envoy-extproc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "nextmoca-envoy-extproc.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "nextmoca-envoy-extproc.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- required "serviceAccount.name is required when serviceAccount.create is false" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "nextmoca-envoy-extproc.image" -}}
{{- if .Values.image.digest }}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest }}
{{- else }}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) }}
{{- end }}
{{- end }}
