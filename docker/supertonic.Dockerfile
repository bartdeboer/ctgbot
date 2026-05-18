FROM python:3.12-slim

ENV PIP_ROOT_USER_ACTION=ignore \
    PYTHONUNBUFFERED=1 \
    HF_HUB_DISABLE_XET=1

RUN apt-get update \
 && apt-get install -y --no-install-recommends libsndfile1 ffmpeg \
 && rm -rf /var/lib/apt/lists/* \
 && pip install --no-cache-dir supertonic

WORKDIR /app
