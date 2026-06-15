FROM ctgbot-go-node-python-base:latest

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
