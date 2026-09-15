FROM ctgbot-go-node-python-base:latest

# Native release, not the npm launcher: the existing PID wrapper must exec the
# actual agent. Keep the complete release layout and disable runtime auto-update.
ARG TARGETARCH
ARG COPILOT_VERSION=1.0.83
RUN set -eu; \
    case "${TARGETARCH:-amd64}" in amd64) arch=x64 ;; arm64) arch=arm64 ;; *) echo 'Unsupported Copilot architecture' >&2; exit 1 ;; esac; \
    name="copilot-linux-${arch}.tar.gz"; \
    base="https://github.com/github/copilot-cli/releases/download/v${COPILOT_VERSION}"; \
    mkdir -p /tmp/copilot-release; \
    cd /tmp/copilot-release; \
    curl --fail --location --silent --show-error "${base}/${name}" -o "$name"; \
    curl --fail --location --silent --show-error "${base}/SHA256SUMS.txt" -o SHA256SUMS.txt; \
    awk -v file="$name" '$2 == file || $2 == "*" file { print }' SHA256SUMS.txt > selected.sha256; \
    test "$(wc -l < selected.sha256)" -eq 1; \
    sha256sum --check selected.sha256; \
    tar -xzf "$name" -C /usr/local/bin; \
    chmod 0755 /usr/local/bin/copilot; \
    rm -rf /tmp/copilot-release

ENV COPILOT_AUTO_UPDATE=false
WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
