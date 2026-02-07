{{/*
生成应用名称
*/}}
{{- define "hexagon.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
生成完整名称
*/}}
{{- define "hexagon.fullname" -}}
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
通用标签
*/}}
{{- define "hexagon.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{ include "hexagon.selectorLabels" . }}
{{- end }}

{{/*
选择器标签
*/}}
{{- define "hexagon.selectorLabels" -}}
app.kubernetes.io/name: {{ include "hexagon.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Qdrant 连接地址
内置组件启用时指向内部 Service，否则使用 external 配置
*/}}
{{- define "hexagon.qdrantURL" -}}
{{- if .Values.qdrant.enabled }}
{{- printf "http://%s-qdrant:%v" (include "hexagon.fullname" .) .Values.qdrant.service.httpPort }}
{{- else }}
{{- .Values.external.qdrant.url }}
{{- end }}
{{- end }}

{{/*
Redis 连接地址
*/}}
{{- define "hexagon.redisURL" -}}
{{- if .Values.redis.enabled }}
{{- printf "%s-redis:%v" (include "hexagon.fullname" .) .Values.redis.service.port }}
{{- else }}
{{- .Values.external.redis.url }}
{{- end }}
{{- end }}

{{/*
Redis 密码
*/}}
{{- define "hexagon.redisPassword" -}}
{{- if .Values.redis.enabled }}
{{- .Values.redis.password | default "" }}
{{- else }}
{{- .Values.external.redis.password | default "" }}
{{- end }}
{{- end }}

{{/*
PostgreSQL DSN
*/}}
{{- define "hexagon.postgresDSN" -}}
{{- if .Values.postgres.enabled }}
{{- printf "postgres://%s:%s@%s-postgres:%v/%s?sslmode=disable" .Values.postgres.auth.username .Values.postgres.auth.password (include "hexagon.fullname" .) .Values.postgres.service.port .Values.postgres.auth.database }}
{{- else }}
{{- .Values.external.postgres.dsn }}
{{- end }}
{{- end }}
