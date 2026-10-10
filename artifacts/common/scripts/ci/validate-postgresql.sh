#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
source deploy/platform/versions.env
case "$(uname -s)" in Darwin) tool_os=darwin ;; Linux) tool_os=linux ;; *) exit 1 ;; esac
case "$(uname -m)" in arm64|aarch64) tool_arch=arm64 ;; x86_64|amd64) tool_arch=amd64 ;; *) exit 1 ;; esac
tools_dir="$PWD/.cache/observability-tools"
export PATH="$tools_dir/helm-$HELM_VERSION/$tool_os-$tool_arch:$tools_dir/kubeconform-$KUBECONFORM_VERSION:$tools_dir/kube-linter-$KUBE_LINTER_VERSION:$PATH"
for command in helm kubeconform kube-linter python3 shellcheck; do
  command -v "$command" >/dev/null || { echo "$command is required; run validate-observability.sh first to prepare pinned tools" >&2; exit 1; }
done

for script in scripts/postgresql/*.sh scripts/postgresql/tests/*.sh scripts/ci/{build-image,publish-image,split-patroni-images}.sh scripts/ci/tests/image*.sh; do
  sh -n "$script"
done
shellcheck -x scripts/postgresql/*.sh scripts/postgresql/tests/*.sh scripts/ci/{build-image,publish-image,split-patroni-images}.sh scripts/ci/tests/image*.sh
sh scripts/postgresql/tests/backup-restore_test.sh
sh scripts/ci/tests/image-artifacts_test.sh
sh scripts/ci/tests/validate-postgresql_test.sh
python3 scripts/ci/tests/postgresql-contracts_test.py

rendered_dir=.cache/rendered/postgresql
mkdir -p "$rendered_dir"
validation_tag=0000000000000000000000000000000000000000
helm lint deploy/helm/postgresql-patroni --strict \
  -f deploy/platform/values/postgresql-patroni.yaml --set-string "image.tag=$validation_tag"
helm template postgresql-patroni deploy/helm/postgresql-patroni --namespace platform \
  -f deploy/platform/values/postgresql-patroni.yaml --set-string "image.tag=$validation_tag" \
  > "$rendered_dir/patroni-resources.yaml"
kubeconform -strict -summary -kubernetes-version 1.35.0 "$rendered_dir/patroni-resources.yaml" deploy/postgresql/*.yaml
kube-linter lint "$rendered_dir/patroni-resources.yaml" deploy/postgresql/*.yaml

POSTGRES_CLIENT_IMAGE="$POSTGRES_CLIENT_IMAGE" python3 - <<'PY'
import os
from pathlib import Path
import yaml
root = Path(".cache/rendered/postgresql")
objects = list(yaml.safe_load_all((root / "patroni-resources.yaml").read_text()))
assert not any(obj["kind"] in ("Secret", "CustomResourceDefinition") for obj in objects)
config = next(obj for obj in objects if obj["kind"] == "ConfigMap")["data"]["patroni.yaml"]
cfg = yaml.safe_load(config)
assert cfg["kubernetes"]["use_endpoints"] is False
assert cfg["kubernetes"]["role_label"] == "role"
assert cfg["postgresql"]["remove_data_directory_on_rewind_failure"] is False
assert cfg["postgresql"]["remove_data_directory_on_diverged_timelines"] is False
assert cfg["bootstrap"]["dcs"]["ttl"] >= cfg["bootstrap"]["dcs"]["loop_wait"] + 2 * cfg["bootstrap"]["dcs"]["retry_timeout"]
sts = next(obj for obj in objects if obj["kind"] == "StatefulSet")["spec"]
assert sts["replicas"] == 3 and sts["podManagementPolicy"] == "Parallel"
assert sts["updateStrategy"]["type"] == "OnDelete"
assert sts["volumeClaimTemplates"][0]["spec"]["resources"]["requests"]["storage"] == "2Gi"
container = sts["template"]["spec"]["containers"][0]
assert container["readinessProbe"]["httpGet"]["path"] == "/readiness"
assert container["livenessProbe"]["httpGet"]["path"] == "/liveness"
services = {obj["metadata"]["name"]: obj for obj in objects if obj["kind"] == "Service"}
assert services["postgresql-primary"]["spec"]["selector"]["role"] == "primary"
assert services["postgresql-replicas"]["spec"]["selector"]["role"] == "replica"
for path in Path("deploy/postgresql").glob("*.yaml"):
    obj = yaml.safe_load(path.read_text())
    assert obj["metadata"]["namespace"] == "platform"
    assert obj["kind"] not in ("Secret", "ConfigMap")
    spec = obj["spec"]
    if obj["kind"] == "CronJob":
        assert spec["concurrencyPolicy"] == "Forbid" and spec["timeZone"] == "Etc/UTC"
        spec = spec["jobTemplate"]["spec"]
    if obj["kind"] in ("CronJob", "Job"):
        spec = spec["template"]["spec"]
    for c in spec.get("containers", []):
        assert c["image"] == os.environ["POSTGRES_CLIENT_IMAGE"]
        assert all(e["secretRef"]["name"] == "postgresql-maintenance-auth" for e in c.get("envFrom", []))
(root / "patroni.yaml").write_text(config)
print("PostgreSQL/Patroni manifest contracts passed; native config validation runs in image build")
PY
go test ./internal/config ./internal/postgres ./migrations ./cmd/migrate
echo 'PostgreSQL static validation passed (not a live HA/restore acceptance)'
