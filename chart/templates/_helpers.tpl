{{/*
Common labels
*/}}
{{- define "ainsel.labels" -}}
app.kubernetes.io/part-of: ainsel
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
{{- end }}

{{/*
Component labels
*/}}
{{- define "ainsel.componentLabels" -}}
{{ include "ainsel.labels" . }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Selector labels for a component
*/}}
{{- define "ainsel.selectorLabels" -}}
app.kubernetes.io/name: ainsel-{{ .component }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
*/}}
