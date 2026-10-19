#!/usr/bin/env bash
set -euo pipefail
if [[ $# != 3 ]]; then
  echo "Usage: bash 02-interviews-and-practice/artifacts/init.sh <level 02|03> <topic 01..10> <task 01..NN>" >&2
  exit 2
fi
python3 - "$(cd "$(dirname "$0")/../.." && pwd)" "$1" "$2" "$3" <<'PYTHON'
from pathlib import Path
import json
import re
import shutil
import sys
import tempfile

root = Path(sys.argv[1])
level_number, topic_number, task_number = sys.argv[2:5]
levels = {"02": "02-interviews-and-practice", "03": "03-advanced"}
if level_number not in levels:
    raise SystemExit("Practice is available only at levels 02 and 03; level 01 is theory only")
if not re.fullmatch(r"0[1-9]|10", topic_number) or not re.fullmatch(r"[0-9]{2}", task_number):
    raise SystemExit("Use two-digit topic and task numbers")
topics = [p for p in (root / levels[level_number]).glob(topic_number + "-*") if p.is_dir()]
if len(topics) != 1:
    raise SystemExit("Topic not found")
tasks = [p for p in topics[0].glob(task_number + "-*") if p.is_dir()]
if len(tasks) != 1:
    raise SystemExit("Task not found")
task = tasks[0]
artifacts = root / "02-interviews-and-practice" / "artifacts"
profiles = json.loads((artifacts / "profiles.json").read_text())
profile = profiles.get(str(task.relative_to(root)))
if profile is None:
    raise SystemExit("No initialization profile configured for " + str(task.relative_to(root)))
workspace = task / "workspace"
if workspace.exists() or workspace.is_symlink():
    raise SystemExit("Workspace already exists; your files were not changed: " + str(workspace))
common = artifacts / "common"
base = [".dockerignore", "env.example", ".golangci.yml", "Dockerfile", "Makefile",
        "go.mod", "go.sum", "cmd", "internal", "migrations", "deploy", "scripts", "containers"]
ignore = shutil.ignore_patterns(".env", "*.pem", "*.key", "node_modules", ".venv", ".terraform", "*.tfstate", "*.tfstate.*")
with tempfile.TemporaryDirectory(prefix=".init-", dir=task) as staging:
    destination = Path(staging) / "workspace"
    destination.mkdir()
    if profile == "06":
        base = ["deploy/kafka-lab"]
    if profile not in {"basic", "02-extra", "09"}:
        for name in base:
            source = common / name
            if source.is_dir():
                shutil.copytree(source, destination / name, ignore=ignore)
            elif source.is_file():
                shutil.copy2(source, destination / name)
        if profile == "00":
            # The application starter uses only the original source inventory.
            shutil.rmtree(destination)
            destination.mkdir()
            overrides = artifacts / "overrides" / "00"
            for relative in (overrides / "files.txt").read_text().splitlines():
                source = overrides / relative
                if not source.exists():
                    source = common / relative
                target = destination / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, target)
        elif profile != "06":
            shutil.copytree(common / "docs", destination / "docs")
    (destination / ".gitignore").write_text(".env\n**/.env\n*.tfstate\n*.tfstate.*\n.terraform/\n")
    (destination / "TASK.md").write_text("[Условие практики](../README.md).\n")
    destination.rename(workspace)
print("Practice initialized: " + str(workspace))
print("cd " + str(workspace))
PYTHON
