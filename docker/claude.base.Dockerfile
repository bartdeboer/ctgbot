FROM ctgbot-go-node-python-base:latest

ENV DEBIAN_FRONTEND=noninteractive

RUN npm install -g @anthropic-ai/claude-code \
    && claude --version

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
