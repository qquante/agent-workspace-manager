FROM node:24-bookworm-slim

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

ENV DEBIAN_FRONTEND=noninteractive \
    HERMES_HOME=/home/stola/.hermes \
    XDG_CONFIG_HOME=/home/stola/.config \
    XDG_DATA_HOME=/home/stola/.local/share \
    STOLA_MCP_MANIFEST=/opt/stola/manifests/mcp/default.yaml \
    STOLA_PROVIDERS_MANIFEST=/opt/stola/manifests/providers/default.yaml \
    PATH=/home/stola/.local/bin:/home/stola/bin:/usr/local/bin:/usr/local/sbin:/usr/sbin:/usr/bin:/sbin:/bin

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bash \
        build-essential \
        ca-certificates \
        curl \
        fd-find \
        ffmpeg \
        git \
        jq \
        less \
        nano \
        openssh-client \
        procps \
        python3 \
        python3-pip \
        python3-venv \
        ripgrep \
        sqlite3 \
        tini \
        unzip \
        xz-utils \
    && ln -sf /usr/bin/fdfind /usr/local/bin/fd \
    && rm -rf /var/lib/apt/lists/*

RUN python3 -m pip install --no-cache-dir --break-system-packages uv litellm

RUN npm install -g opencode-ai openclaw@latest

RUN curl -fsSL https://hermes-agent.nousresearch.com/install.sh -o /tmp/hermes-install.sh \
    && bash /tmp/hermes-install.sh --skip-setup --skip-browser --non-interactive \
    && rm -f /tmp/hermes-install.sh

RUN groupadd --gid 10001 stola \
    && useradd --uid 10001 --gid stola --create-home --shell /bin/bash stola \
    && mkdir -p /workspace /opt/stola/manifests/mcp /opt/stola/manifests/providers /home/stola/.config /home/stola/.local/share \
    && chown -R stola:stola /workspace /opt/stola /home/stola

COPY manifests/mcp/default.yaml /opt/stola/manifests/mcp/default.yaml
COPY manifests/providers/default.yaml /opt/stola/manifests/providers/default.yaml

RUN command -v hermes \
    && command -v opencode \
    && command -v openclaw \
    && command -v uvx \
    && command -v litellm \
    && chown -R stola:stola /opt/stola

USER stola
WORKDIR /workspace

LABEL org.opencontainers.image.title="stola-agent-base" \
      org.opencontainers.image.description="Stola Docker-first agent workspace base with Hermes, OpenCode, OpenClaw, MCP manifest, and provider catalog" \
      ai.stola.image.kind="agent-base"

ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["sh", "-lc", "while true; do sleep 3600; done"]
