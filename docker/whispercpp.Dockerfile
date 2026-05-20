FROM ghcr.io/ggml-org/whisper.cpp:main-cuda

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
