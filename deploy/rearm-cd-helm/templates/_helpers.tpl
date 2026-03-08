{{/*
Resolve the service account name.
Uses rbac.serviceAccountName if set, otherwise defaults to <namespace>-<release>-rearm-cd.
*/}}
{{- define "rearm-cd.serviceAccountName" -}}
{{- if .Values.rbac.serviceAccountName -}}
{{- .Values.rbac.serviceAccountName -}}
{{- else -}}
{{- printf "%s-%s-rearm-cd" .Release.Namespace .Release.Name -}}
{{- end -}}
{{- end -}}
