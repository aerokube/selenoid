#!/bin/bash

set -e

docker login -u="$DOCKER_USERNAME" -p="$DOCKER_PASSWORD"
docker buildx build --push -t "$GITHUB_REPOSITORY" -t "$GITHUB_REPOSITORY:$1" -t "selenoid/hub:$1" --platform linux/amd64,linux/arm64 .
