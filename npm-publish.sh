#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLATFORM_DIR="${SCRIPT_DIR}/npm-platforms"
MAIN_PKG_DIR="${SCRIPT_DIR}/npm"
ROOT_PKG="${SCRIPT_DIR}/package.json"
PKG_ROOT="${SCRIPT_DIR}/package.json"

ACCESS_FLAG="${NPM_ACCESS:-public}"
VERSION_ARG=""
DRY_RUN=0

for arg in "$@"; do
	case "$arg" in
		--version:*)
			VERSION_ARG="${arg#--version:}"
			;;
		--dry-run)
			DRY_RUN=1
			;;
	esac
done

update_version() {
	local file="$1"
	local ver="$2"
	node -e "const fs=require('fs');const f='$file';const v='$ver';const j=JSON.parse(fs.readFileSync(f,'utf8'));j.version=v;if(j.optionalDependencies){for(const k of Object.keys(j.optionalDependencies)){j.optionalDependencies[k]=v;}}if(j.dependencies && j.dependencies['oi-proxy']){j.dependencies['oi-proxy']=v;}fs.writeFileSync(f,JSON.stringify(j,null,2)+'\n');"
	echo "[proxy-npm] set version $ver in $(basename "$file")"
}

if [ -n "${VERSION_ARG}" ]; then
	update_version "${MAIN_PKG_DIR}/package.json" "${VERSION_ARG}"
	update_version "${PLATFORM_DIR}/proxy-darwin-arm64/package.json" "${VERSION_ARG}"
	update_version "${PLATFORM_DIR}/proxy-darwin-amd64/package.json" "${VERSION_ARG}"
	update_version "${PLATFORM_DIR}/proxy-linux-amd64/package.json" "${VERSION_ARG}"
	update_version "${PLATFORM_DIR}/proxy-win32-amd64/package.json" "${VERSION_ARG}"
	if [ -f "${SCRIPT_DIR}/package.json" ]; then
		update_version "${SCRIPT_DIR}/package.json" "${VERSION_ARG}" || true
	fi
fi

ensure_login() {
	if npm whoami >/dev/null 2>&1; then
		return
	fi
	echo "[proxy-npm] npm is not authenticated. Launching npm login..."
	npm login
}

if [ "${DRY_RUN}" -eq 0 ]; then
	ensure_login
else
	echo "[proxy-npm] dry-run: skipping npm login"
fi

echo "[proxy-npm] building binaries..."
"${SCRIPT_DIR}/npm-build.sh"

publish_pkg() {
	local pkg_dir="$1"
	local skip_build="${2:-}"
	echo "[proxy-npm] publishing $(basename "${pkg_dir}")"
	(
		cd "${pkg_dir}"
		if [ "${DRY_RUN}" -eq 1 ]; then
			echo "[proxy-npm] [dry-run] npm publish --access \"${ACCESS_FLAG}\""
			return
		fi
		if [ -n "${skip_build}" ]; then
			SKIP_PLATFORM_BUILD=1 npm publish --access "${ACCESS_FLAG}"
		else
			npm publish --access "${ACCESS_FLAG}"
		fi
	)
}

publish_pkg "${PLATFORM_DIR}/proxy-darwin-arm64"
publish_pkg "${PLATFORM_DIR}/proxy-darwin-amd64"
publish_pkg "${PLATFORM_DIR}/proxy-linux-amd64"
publish_pkg "${PLATFORM_DIR}/proxy-win32-amd64"

publish_pkg "${MAIN_PKG_DIR}" "skip_build"

echo "[proxy-npm] publish completed."

