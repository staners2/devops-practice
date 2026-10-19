#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../.."
source deploy/platform/versions.env
for command in curl tar unzip jq go; do
  command -v "$command" >/dev/null || { echo "$command is required" >&2; exit 1; }
done
case "$(uname -s)" in Darwin) tool_os=darwin ;; Linux) tool_os=linux ;; *) echo 'Run validation in Linux CI or on macOS' >&2; exit 1 ;; esac
case "$(uname -m)" in arm64|aarch64) tool_arch=arm64 ;; x86_64|amd64) tool_arch=amd64 ;; *) echo 'Unsupported architecture' >&2; exit 1 ;; esac
tools_dir="$PWD/.cache/observability-tools"
mkdir -p "$tools_dir"

download_tar() {
  local url=$1 destination=$2 executable=$3
  if [[ ! -x "$destination/$executable" ]]; then
    mkdir -p "$destination"
    curl --fail --silent --show-error --location --retry 3 "$url" -o "$destination/release.tar.gz"
    tar -xzf "$destination/release.tar.gz" -C "$destination"
  fi
}

download_tar "https://get.helm.sh/helm-v${HELM_VERSION}-${tool_os}-${tool_arch}.tar.gz" "$tools_dir/helm-$HELM_VERSION" "$tool_os-$tool_arch/helm"
download_tar "https://github.com/yannh/kubeconform/releases/download/v${KUBECONFORM_VERSION}/kubeconform-${tool_os}-${tool_arch}.tar.gz" "$tools_dir/kubeconform-$KUBECONFORM_VERSION" kubeconform
linter_platform=$tool_os
if [[ $tool_arch == arm64 ]]; then linter_platform="${tool_os}_arm64"; fi
download_tar "https://github.com/stackrox/kube-linter/releases/download/v${KUBE_LINTER_VERSION}/kube-linter-${linter_platform}.tar.gz" "$tools_dir/kube-linter-$KUBE_LINTER_VERSION" kube-linter
download_tar "https://github.com/VictoriaMetrics/VictoriaMetrics/releases/download/${VICTORIA_METRICS_VERSION}/vmutils-${tool_os}-${tool_arch}-${VICTORIA_METRICS_VERSION}.tar.gz" "$tools_dir/vmutils-$VICTORIA_METRICS_VERSION" vmalert-tool-prod
download_tar "https://github.com/prometheus/alertmanager/releases/download/${ALERTMANAGER_VERSION}/alertmanager-${ALERTMANAGER_VERSION#v}.${tool_os}-${tool_arch}.tar.gz" "$tools_dir/alertmanager-$ALERTMANAGER_VERSION" "alertmanager-${ALERTMANAGER_VERSION#v}.${tool_os}-${tool_arch}/amtool"
alloy_bin="$tools_dir/alloy-$ALLOY_VERSION/alloy-$tool_os-$tool_arch"
if [[ ! -x "$alloy_bin" ]]; then
  mkdir -p "$tools_dir/alloy-$ALLOY_VERSION"
  curl --fail --silent --show-error --location --retry 3 "https://github.com/grafana/alloy/releases/download/${ALLOY_VERSION}/alloy-${tool_os}-${tool_arch}.zip" -o "$tools_dir/alloy-$ALLOY_VERSION/release.zip"
  unzip -oq "$tools_dir/alloy-$ALLOY_VERSION/release.zip" -d "$tools_dir/alloy-$ALLOY_VERSION"
  chmod +x "$alloy_bin"
fi
export PATH="$tools_dir/helm-$HELM_VERSION/$tool_os-$tool_arch:$tools_dir/kubeconform-$KUBECONFORM_VERSION:$tools_dir/kube-linter-$KUBE_LINTER_VERSION:$PATH"
export AMTOOL="$tools_dir/alertmanager-$ALERTMANAGER_VERSION/alertmanager-${ALERTMANAGER_VERSION#v}.${tool_os}-${tool_arch}/amtool"
export ALERTMANAGER="${AMTOOL%/amtool}/alertmanager"
export VMALERT_TOOL="$tools_dir/vmutils-$VICTORIA_METRICS_VERSION/vmalert-tool-prod"

go test ./internal/observability ./internal/httpapi ./internal/processor
helm repo add vm https://victoriametrics.github.io/helm-charts --force-update >/dev/null
helm repo add grafana-community https://grafana-community.github.io/helm-charts --force-update >/dev/null
helm repo add grafana https://grafana.github.io/helm-charts --force-update >/dev/null
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts --force-update >/dev/null
helm repo update vm grafana-community grafana prometheus-community >/dev/null

rendered_dir=.cache/rendered/observability
mkdir -p "$rendered_dir"
while read -r release repo chart version; do
  chart_dir=".cache/charts/observability/${chart}-${version}/${chart}"
  if [[ ! -d "$chart_dir" ]]; then
    helm pull "$repo/$chart" --version "$version" --untar --untardir "$(dirname "$chart_dir")"
  fi
  helm lint "$chart_dir" -f "deploy/platform/values/$chart.yaml" --strict
  helm template "$release" "$chart_dir" --namespace monitoring -f "deploy/platform/values/$chart.yaml" > "$rendered_dir/$release.yaml"
  kubeconform -strict -summary -kubernetes-version 1.35.0 "$rendered_dir/$release.yaml"
  kube-linter lint "$rendered_dir/$release.yaml"
done <<VERSIONS
victoriametrics vm victoria-metrics-single $VICTORIA_METRICS_SINGLE_CHART_VERSION
vmagent vm victoria-metrics-agent $VICTORIA_METRICS_AGENT_CHART_VERSION
vmalert vm victoria-metrics-alert $VICTORIA_METRICS_ALERT_CHART_VERSION
grafana grafana-community grafana $GRAFANA_CHART_VERSION
loki grafana-community loki $LOKI_CHART_VERSION
alloy grafana alloy $ALLOY_CHART_VERSION
kube-state-metrics prometheus-community kube-state-metrics $KUBE_STATE_METRICS_CHART_VERSION
node-exporter prometheus-community prometheus-node-exporter $PROMETHEUS_NODE_EXPORTER_CHART_VERSION
postgresql-exporter prometheus-community prometheus-postgres-exporter $POSTGRES_EXPORTER_CHART_VERSION
VERSIONS
./scripts/ci/validate-helm.sh
kubeconform -strict -summary -kubernetes-version 1.35.0 deploy/monitoring/bootstrap/*.yaml
if grep -Eq 'kind: (ServiceMonitor|PodMonitor|PrometheusRule|VM[A-Z])|:[l]atest(["[:space:]]|$)' "$rendered_dir"/*.yaml; then
  echo 'Operators/monitoring CRDs and latest images are forbidden' >&2; exit 1
fi
if grep -REn '^[[:space:]]*(bot_token|chat_id|adminPassword|password):[[:space:]]*[^[:space:]]' deploy/monitoring deploy/platform/values/{grafana,victoria-metrics-alert}.yaml; then
  echo 'Plaintext monitoring credentials are forbidden' >&2; exit 1
fi

"$tools_dir/vmutils-$VICTORIA_METRICS_VERSION/vmagent-prod" -promscrape.config=deploy/monitoring/scrape/vmagent.yaml -promscrape.config.dryRun -remoteWrite.url=http://localhost:8428/api/v1/write
"$tools_dir/vmutils-$VICTORIA_METRICS_VERSION/vmalert-prod" -dryRun -rule='deploy/monitoring/rules/alerts.yaml' -rule='deploy/monitoring/rules/recording.yaml'
"$VMALERT_TOOL" unittest -files='deploy/monitoring/rules/tests/*_test.yaml'
go test ./deploy/monitoring/rules/tests -run 'TestAlertmanager|TestDashboard' -count=1 -v
"$alloy_bin" fmt --test deploy/monitoring/alloy/config.alloy
"$alloy_bin" validate deploy/monitoring/alloy/config.alloy

for dashboard in deploy/monitoring/dashboards/*.json; do
  jq -e '(.uid | type == "string") and (.panels | length > 0) and
    ([.. | objects | select(has("datasource")) | .datasource.uid | select(. != "-- Grafana --")] | all(. == "victoriametrics" or . == "loki")) and
    ([.panels[].targets[]? | has("datasource")] | all)' "$dashboard" >/dev/null
done
jq -e '.uid == "service-overview" and
  (["Частота запросов API","Доля ответов 5xx","Задержка API: p50 / p95 / p99","Запросы в обработке","Доступность зависимостей","Kafka: публикация и обработка","Возраст успешной обработки worker","Последняя успешная обработка","API / worker: CPU","API / worker: память","API / worker: перезапуски за интервал","Логи приложения"] - [.panels[].title] | length == 0)' deploy/monitoring/dashboards/service-overview.json >/dev/null
jq -e '.uid == "kubernetes-operations" and
  (["Node: загрузка CPU","Node: использование памяти","Node: заполнение filesystem","Workloads: desired / available replicas","CPU: requests / limits / usage","Память: requests / limits / usage","Pod: перезапуски за интервал","Jobs: ошибки","Ingress: частота запросов","Ingress: ответы 4xx / 5xx","Scrape targets: состояние","Observability stack: targets"] - [.panels[].title] | length == 0)' deploy/monitoring/dashboards/kubernetes-operations.json >/dev/null
jq -e '.uid == "incident-drilldown" and ([.templating.list[].name] | index("request_id") != null) and (.annotations.list | length > 0)' deploy/monitoring/dashboards/incident-drilldown.json >/dev/null
echo 'Observability validation passed'
