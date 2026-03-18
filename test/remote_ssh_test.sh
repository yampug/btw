#!/bin/bash
set -e

# remote_ssh_test.sh: Integration test for remote search over SSH.
# Requires SSH access to localhost (ssh localhost should work without password).

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
TEST_DIR="/tmp/btw-remote-test"
AGENT_BIN="$HOME/.local/bin/btw-agent"

echo "--- Building btw and btw-agent ---"
cd "$REPO_ROOT"
go build -o bin/btw ./cmd/btw
go build -o bin/btw-agent ./cmd/btw-agent

echo "--- Setting up test project at $TEST_DIR ---"
rm -rf "$TEST_DIR"
mkdir -p "$TEST_DIR/sub"
echo "package main" > "$TEST_DIR/main.go"
echo "# Test Project" > "$TEST_DIR/README.md"
echo "func foo() {}" > "$TEST_DIR/sub/lib.go"
echo "node_modules/" > "$TEST_DIR/.gitignore"
mkdir -p "$TEST_DIR/node_modules"
echo "ignored" > "$TEST_DIR/node_modules/bad.go"

echo "--- Deploying btw-agent to localhost ---"
./bin/btw --remote localhost --deploy-agent --version > /dev/null

echo "--- Running remote walk test ---"
RESULT=$(./bin/btw --remote "localhost:$TEST_DIR" --test-query "README" 2>/dev/null)
echo "Result: $RESULT"

if [[ "$RESULT" == *"$TEST_DIR/README.md"* ]]; then
    echo "✅ Remote walk/search test passed!"
else
    echo "❌ Remote walk/search test failed!"
    exit 1
fi

echo "--- Running remote grep test ---"
# Select 'symbols' tab via -t and query "foo"
RESULT=$(./bin/btw --remote "localhost:$TEST_DIR" -t symbols --test-query "foo" 2>/dev/null)
echo "Result: $RESULT"

if [[ "$RESULT" == *"$TEST_DIR/sub/lib.go"* ]]; then
    echo "✅ Remote symbols test passed!"
else
    echo "❌ Remote symbols test failed!"
    exit 1
fi

echo "--- Cleanup ---"
rm -rf "$TEST_DIR"

echo "--- ALL TESTS PASSED ---"
