FROM python:3.14.5-slim-bookworm@sha256:a9bee15510a364124aa24692899d269835683b883de42f7ebec8c293cf679ccb
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

# apt-пакеты берутся с security updates; Terraform/TFLint закреплены по SHA256.
# hadolint ignore=DL3008
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl unzip jq git openssh-client coreutils nano \
    && rm -rf /var/lib/apt/lists/*
COPY deploy/terraform-lab/versions.env /opt/lab/terraform-versions.env
COPY deploy/ansible-lab/versions.env /opt/lab/ansible-versions.env
COPY scripts/ci/install-terraform-tools.sh /opt/lab/install-tools.sh
RUN bash /opt/lab/install-tools.sh /opt/lab/terraform-versions.env /usr/local/bin \
    && source /opt/lab/ansible-versions.env \
    && python -m pip install --no-cache-dir "ansible-core==$ANSIBLE_CORE_VERSION" "ansible-lint==$ANSIBLE_LINT_VERSION" \
    && ansible-galaxy collection install "community.docker:$COMMUNITY_DOCKER_VERSION" -p /opt/collections \
    && useradd --create-home --uid 1000 lab
ENV ANSIBLE_COLLECTIONS_PATH=/opt/collections \
    EDITOR=nano \
    ANSIBLE_VAULT_EDITOR=nano \
    PYTHONDONTWRITEBYTECODE=1 \
    TF_IN_AUTOMATION=1 \
    CHECKPOINT_DISABLE=1
USER 1000:1000
WORKDIR /workspace
CMD ["bash"]
