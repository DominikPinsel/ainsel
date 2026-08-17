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

{{/*
Local mode: nothing is externally exposed — no hub ingress, no frontend
ingress (or frontend disabled), no MCP ingress (or MCP disabled), and no
OIDC issuer configured. In that situation the hub may safely run without
auth middleware; the chart then auto-enables HUB_ALLOW_INSECURE_NO_AUTH so
a port-forward-only install works with zero auth configuration. Note:
WebhookConnector CRs still get their own ingress via the gateway operator —
keep local mode truly local and do not create connectors on an exposed
cluster while relying on it.
*/}}
{{- define "ainsel.localMode" -}}
{{- $uiExposed := and .Values.ui.enabled .Values.ui.ingress.enabled -}}
{{- $mcpExposed := and .Values.mcp.enabled .Values.mcp.ingress.enabled -}}
{{- if and (not .Values.auth.oidcIssuer) (not .Values.hub.ingress.enabled) (not $uiExposed) (not $mcpExposed) -}}
true
{{- end -}}
{{- end -}}
