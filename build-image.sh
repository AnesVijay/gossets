#!/bin/bash
VERSION="1.0"
IMAGE_NAME="gossets"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

docker build -t ${IMAGE_NAME}:${VERSION} ${SCRIPT_DIR}

echo "✅ Built: ${IMAGE_NAME}:${VERSION}"
