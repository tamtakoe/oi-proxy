#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLATFORM_DIR="${SCRIPT_DIR}/npm-platforms"

pushd "${SCRIPT_DIR}" >/dev/null

TARGETS=(
	"darwin arm64 proxy-darwin-arm64 oi-proxy"
	"darwin amd64 proxy-darwin-amd64 oi-proxy"
	"linux amd64 proxy-linux-amd64 oi-proxy"
	"windows amd64 proxy-win32-amd64 oi-proxy.exe"
)

for target in "${TARGETS[@]}"; do
	read -r GOOS GOARCH PACKAGE_NAME BINARY <<<"${target}"
	PACKAGE_DIR="${PLATFORM_DIR}/${PACKAGE_NAME}"
	OUT_DIR="${PACKAGE_DIR}/bin"
	rm -rf "${OUT_DIR}"
	mkdir -p "${OUT_DIR}"
	OUT_PATH="${OUT_DIR}/${BINARY}"

	echo "[proxy-npm] building ${GOOS}/${GOARCH} -> ${OUT_PATH}"
	GOOS="${GOOS}" GOARCH="${GOARCH}" go build -o "${OUT_PATH}" ./cmd/oi-proxy

	if [[ "${GOOS}" != "windows" ]]; then
		chmod +x "${OUT_PATH}"
	fi
done

echo "[proxy-npm] binaries ready in ${PLATFORM_DIR}"

popd >/dev/null

