{{/*
Names and labels.

One place, because a selector that disagrees with a label by one character is
a Deployment that manages nothing and reports itself healthy.
*/}}

{{- define "fuseone.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "fuseone.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "fuseone.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "fuseone.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ include "fuseone.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: fuseone-agents
{{- end -}}

{{- define "fuseone.selectorLabels" -}}
app.kubernetes.io/name: {{ include "fuseone.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "fuseone.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "fuseone.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "fuseone.image" -}}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) -}}
{{- end -}}

{{- define "fuseone.secretName" -}}
{{- default (printf "%s-secret" (include "fuseone.fullname" .)) .Values.secret.existingSecret -}}
{{- end -}}

{{/*
The environment every process shares.

The database and the master key are never values on a container: they are
references into a Secret, so `kubectl get deployment -o yaml` shows neither.
*/}}
{{/*
The database and nothing else.

For the hook Jobs, which talk to Postgres and never unseal a credential. The
master key seals every stored credential in the installation and losing it means
re-entering all of them; a short-lived pod that never reads it has no business
holding it, for the same reason these Jobs carry no service account token.
*/}}
{{- define "fuseone.databaseEnv" -}}
- name: DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ include "fuseone.secretName" . }}
      key: DATABASE_URL
{{- end -}}

{{/*
Everything a process that serves needs. The API server and the worker unseal
stored credentials to reach connectors and model providers, so they hold the
key; nothing else does.
*/}}
{{- define "fuseone.env" -}}
{{ include "fuseone.databaseEnv" . }}
- name: FUSEONE_MASTER_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "fuseone.secretName" . }}
      key: FUSEONE_MASTER_KEY
{{- end -}}
