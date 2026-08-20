{{/*
Common labels
*/}}
{{- define "kos-fixture.labels" -}}
app.kubernetes.io/name: {{ .Values.appName }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/release-name: {{ .Release.Name }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kos-fixture.selectorLabels" -}}
app.kubernetes.io/name: {{ .Values.appName }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
