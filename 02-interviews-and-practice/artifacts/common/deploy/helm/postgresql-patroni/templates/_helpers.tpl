{{- define "postgresql-patroni.selectorLabels" -}}
app.kubernetes.io/name: postgresql-patroni
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "postgresql-patroni.labels" -}}
{{ include "postgresql-patroni.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}

{{- define "postgresql-patroni.validate" -}}
{{- if ne .Release.Name "postgresql-patroni" -}}
{{- fail "release name must be postgresql-patroni: DCS scope and Service names are fixed" -}}
{{- end -}}
{{- if gt (add .Values.patroni.loopWait (mul 2 .Values.patroni.retryTimeout)) (int64 .Values.patroni.ttl) -}}
{{- fail "Patroni requires loopWait + 2 * retryTimeout <= ttl" -}}
{{- end -}}
{{- end }}
