# Образ PostgreSQL/Patroni

База `postgres:18.6-bookworm` закреплена по multi-platform OCI index digest.
Upstream [метаданные образа](https://github.com/docker-library/repo-info/blob/master/repos/postgres/remote/18.6-bookworm.md)
подтверждают `linux/amd64` и `linux/arm64`, UID/GID `postgres` 999 и
`/usr/lib/postgresql/18/bin`. Dockerfile проверяет версии PostgreSQL,
`psql`, `pg_basebackup`, `pg_rewind`, Patroni и `patronictl` при сборке.

Patroni 4.1.5 запускается как PID 1 от UID/GID 999 вместо upstream
`docker-entrypoint.sh`: PostgreSQL должен инициализировать и запускать Patroni,
а не второй entrypoint. Python-пакеты устанавливаются в `/opt/patroni`.

`requirements.lock` — generated artifact; меняйте `requirements.in` и
пересоздавайте lock из корня репозитория:

```sh
uv pip compile containers/patroni/requirements.in \
  --generate-hashes --universal --python-version 3.11 \
  --no-emit-index-url --no-emit-find-links \
  --output-file containers/patroni/requirements.lock
uv run --no-project --python 3.11 \
  --with-requirements containers/patroni/requirements.lock \
  python -B -m unittest discover -s containers/patroni/tests -p 'test_*.py' -v
```

Dockerfile устанавливает все зависимости с `--require-hashes`.
Перед сборкой `scripts/ci/validate-postgresql.sh` извлекает Patroni config из
render собственного chart. CI копирует `.cache/rendered/postgresql/patroni.yaml`
в игнорируемый `containers/patroni/validation.yaml`. Dockerfile запускает
`patroni --validate-config --ignore-listen-port` с этим generated input и
реальный PostgreSQL bootstrap self-test. Локальная сборка требует того же
шага подготовки; пустой/отсутствующий input не обходится fallback config.
CI материализует amd64/arm64 из одного OCI index через Skopeo в отдельные
Docker archives и проверяет architecture каждого. Trivy и Syft работают
с этими одиночными образами; опубликованный index остаётся исходным.
Локальные unit tests проверяют quoting, параметры пароля и redaction;
они не доказывают bootstrap на PostgreSQL или HA. Оба целевых Linux
architecture должны отдельно пройти image build/import и cluster acceptance.

`post-bootstrap.py` получает только connection URL из callback Patroni;
`PGPASSFILE` предоставляет Patroni. Пароль роли `devops` поступает через
`DEVOPS_PASSWORD` из Secret, не из argv. `ClientCursor` нужен для безопасной
параметризации PostgreSQL utility statement `ALTER ROLE`; identifiers
формируются через `psycopg.sql.Identifier`.
