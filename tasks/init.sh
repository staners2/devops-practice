#!/usr/bin/env bash
set -euo pipefail
if [[ $# != 1 ]]; then
  echo "Usage: bash tasks/init.sh <00..09|02-extra>" >&2
  exit 2
fi
python3 - "$(cd "$(dirname "$0")/.." && pwd)" "$1" <<'PYTHON'
from pathlib import Path
import shutil
import sys
import tempfile

root = Path(sys.argv[1])
module = sys.argv[2]
if module not in {f"{i:02}" for i in range(10)} | {"02-extra"}:
    raise SystemExit("Unknown module: " + module)
task = root / "tasks" / module
workspace = task / "workspace"
if workspace.exists() or workspace.is_symlink():
    raise SystemExit("Workspace already exists; your files were not changed: " + str(workspace))
common = root / "artifacts" / "common"
base = [".dockerignore", ".env.example", ".golangci.yml", "Dockerfile", "Makefile",
        "go.mod", "go.sum", "cmd", "internal", "migrations", "deploy", "scripts", "containers"]
ignore = shutil.ignore_patterns(".env", "*.pem", "*.key", "node_modules", ".venv", ".terraform", "*.tfstate", "*.tfstate.*")
with tempfile.TemporaryDirectory(prefix=".init-", dir=task) as staging:
    destination = Path(staging) / "workspace"
    destination.mkdir()
    if module == "06":
        base = ["deploy/kafka-lab"]
    if module not in {"02-extra", "09"}:
        for name in base:
            source = common / name
            if source.is_dir():
                shutil.copytree(source, destination / name, ignore=ignore)
            elif source.is_file():
                shutil.copy2(source, destination / name)
        if module == "00":
            # Module 00 uses the initial application, before later infrastructure changes.
            shutil.rmtree(destination)
            destination.mkdir()
            overrides = root / "artifacts" / "overrides" / "00"
            for relative in (overrides / "files.txt").read_text().splitlines():
                source = overrides / relative
                if not source.exists():
                    source = common / relative
                target = destination / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, target)
        elif module != "06":
            shutil.copytree(common / "docs", destination / "docs")
    (destination / ".gitignore").write_text(".env\n**/.env\n*.tfstate\n*.tfstate.*\n.terraform/\n")
    (destination / "TASK.md").write_text("[Условие практики](../README.md).\n")
    destination.rename(workspace)
print("Practice initialized: " + str(workspace))
print("cd " + str(workspace))
PYTHON
