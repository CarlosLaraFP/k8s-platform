{{/*
Generate a full name for resources
*/}}
{{- define "api-server.fullname" -}}
{{- printf "%s" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end }}