FROM ctgbot-go-node-python-cuda-base:latest

# Runtime libraries only; the browser remains a separately managed artifact.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        libnspr4 \
        libnss3 \
        libatk1.0-0 \
        libatk-bridge2.0-0 \
        libdbus-1-3 \
        libcups2 \
        libxcb1 \
        libxkbcommon0 \
        libasound2 \
        libgbm1 \
        libx11-6 \
        libxext6 \
        libcairo2 \
        libpango-1.0-0 \
        libxcomposite1 \
        libxdamage1 \
        libxfixes3 \
        libxrandr2 \
        libatspi2.0-0 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

ARG CODEX_VERSION=latest

RUN curl -fsSL --retry 5 --retry-all-errors --retry-delay 2 --retry-max-time 120 https://chatgpt.com/codex/install.sh -o /tmp/install-codex.sh \
    && chmod +x /tmp/install-codex.sh \
    && if [ -n "${CODEX_VERSION}" ] && [ "${CODEX_VERSION}" != "latest" ]; then \
        CODEX_NON_INTERACTIVE=true CODEX_INSTALL_DIR=/usr/local/bin /tmp/install-codex.sh --release "${CODEX_VERSION}"; \
    else \
        CODEX_NON_INTERACTIVE=true CODEX_INSTALL_DIR=/usr/local/bin /tmp/install-codex.sh; \
    fi \
    && rm -f /tmp/install-codex.sh \
    && mkdir -p /opt/codex \
    && codex_target="$(readlink -f /usr/local/bin/codex)" \
    && cp -a "$(dirname "${codex_target}")/." /opt/codex/ \
    && ln -sf /opt/codex/codex /usr/local/bin/codex \
    && chmod -R a+rX /opt/codex \
    && chmod 755 /usr/local/bin/codex \
    && codex --version

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
