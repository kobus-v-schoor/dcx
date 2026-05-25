#!/usr/bin/env bash
# integration-test.sh runs end-to-end integration tests for dcx against the
# test/ devcontainer setup. It verifies the up/down cycle, mount injection,
# environment variable injection, and proxy setup inside the container.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DCX_BIN="${SCRIPT_DIR}/../dcx"
TEST_DIR="${SCRIPT_DIR}"
export DCX_TEST_MARKER_PATH="${TEST_DIR}/integration-test-marker.txt"

# cleanup stops and removes the devcontainer on exit.
cleanup() {
	local code=$?
	echo "Cleaning up..."
	if [ -f "${DCX_BIN}" ]; then
		cd "${TEST_DIR}"
		"${DCX_BIN}" down --workspace-folder . 2>/dev/null || true
	fi
	exit "${code}"
}
trap cleanup EXIT

# Check prerequisites.
if ! command -v devcontainer >/dev/null 2>&1; then
	echo "ERROR: devcontainer CLI not found. Install it with: npm install -g @devcontainers/cli"
	exit 1
fi

# Build dcx.
echo "Building dcx..."
cd "${SCRIPT_DIR}/.."
go build -o dcx ./cmd/dcx

cd "${TEST_DIR}"

echo "=== Test: dcx up ==="
"${DCX_BIN}" up --workspace-folder .

echo "=== Verify container is running ==="
CONTAINER_ID=$(docker ps -q -f "label=devcontainer.local_folder=$(pwd)")
if [ -z "${CONTAINER_ID}" ]; then
	echo "ERROR: No running devcontainer found for $(pwd)"
	exit 1
fi
echo "Container running: ${CONTAINER_ID}"

echo "=== Test: mounts ==="
RESULT=$("${DCX_BIN}" exec --workspace-folder . -- cat /opt/dcx/test/integration-test-marker.txt)
if [ "${RESULT}" != "dcx-integration-test-marker" ]; then
	echo "ERROR: Mount test failed. Expected 'dcx-integration-test-marker', got '${RESULT}'"
	exit 1
fi
echo "Mount OK"

echo "=== Test: env var injection ==="
if ! "${DCX_BIN}" exec --workspace-folder . -- env | grep -q "DCX_TEST_ENV=integration-test-value"; then
	echo "ERROR: Env var DCX_TEST_ENV not found in container"
	exit 1
fi
echo "Env var injection OK"

echo "=== Test: proxy setup ==="
# Run all proxy checks in a single exec session because the proxy tears down
# (and removes the injected CA cert) when the exec session ends.
if ! "${DCX_BIN}" exec --workspace-folder . -- sh -c '
	ENV_OUT=$(env)
	if ! echo "${ENV_OUT}" | grep -q "GH_TOKEN=dummy"; then
		echo "missing GH_TOKEN"
		exit 1
	fi
	if ! echo "${ENV_OUT}" | grep -q "HTTP_PROXY="; then
		echo "missing HTTP_PROXY"
		exit 1
	fi
	if ! echo "${ENV_OUT}" | grep -q "HTTPS_PROXY="; then
		echo "missing HTTPS_PROXY"
		exit 1
	fi
	if ! ls /usr/local/share/ca-certificates/dcx-proxy-ca-*.crt >/dev/null 2>&1; then
		echo "missing proxy CA cert"
		exit 1
	fi
'; then
	echo "ERROR: Proxy setup test failed"
	exit 1
fi
echo "Proxy setup OK"

echo "=== Test: dcx down ==="
"${DCX_BIN}" down --workspace-folder .

echo "=== Verify container removed ==="
if docker ps -a -q -f "label=devcontainer.local_folder=$(pwd)" | grep -q .; then
	echo "ERROR: Container still exists after down"
	exit 1
fi
echo "Container removed OK"

echo ""
echo "All integration tests passed!"
