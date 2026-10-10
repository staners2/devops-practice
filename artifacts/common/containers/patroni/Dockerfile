FROM postgres:18.6-bookworm@sha256:3725f4e2499eef5134592b3b4ab79a543ed7f8e533b05b5b637af926630f6650
ARG VCS_REF
ARG SOURCE_URL
LABEL org.opencontainers.image.revision=$VCS_REF \
      org.opencontainers.image.source=$SOURCE_URL

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        python3=3.11.2-1+b1 python3-venv=3.11.2-1+b1 ca-certificates=20230311+deb12u1 \
    && rm -rf /var/lib/apt/lists/* \
    && python3 -m venv /opt/patroni

ENV PATH="/opt/patroni/bin:/usr/lib/postgresql/18/bin:${PATH}" \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

COPY requirements.lock /opt/patroni/requirements.lock
RUN python -m pip install --no-cache-dir --require-hashes -r /opt/patroni/requirements.lock \
    && python -m pip check \
    && patroni --version \
    && patronictl --version \
    && postgres --version \
    && psql --version \
    && pg_basebackup --version \
    && pg_rewind --version

COPY post-bootstrap.py /opt/patroni/post-bootstrap.py
USER 999:999
RUN --mount=type=bind,source=validation.yaml,target=/tmp/patroni-validation.yaml,ro \
    PATRONI_NAME=config-check \
    PATRONI_POSTGRESQL_CONNECT_ADDRESS=192.0.2.10:5432 \
    PATRONI_RESTAPI_CONNECT_ADDRESS=192.0.2.10:8008 \
    PATRONI_RESTAPI_PASSWORD=validation_only \
    patroni --validate-config --ignore-listen-port /tmp/patroni-validation.yaml
RUN --mount=type=bind,source=tests,target=/tests,ro /bin/sh /tests/bootstrap.sh
STOPSIGNAL SIGTERM
ENTRYPOINT ["/opt/patroni/bin/patroni"]
CMD ["/etc/patroni/patroni.yaml"]
