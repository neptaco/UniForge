#!/bin/bash

# Create a test Unity project structure
TEST_DIR="test-project"

echo "Setting up test Unity project..."

# Create project directories
mkdir -p "$TEST_DIR/ProjectSettings"
mkdir -p "$TEST_DIR/Assets"

# Create a mock ProjectVersion.txt
cat > "$TEST_DIR/ProjectSettings/ProjectVersion.txt" << EOF
m_EditorVersion: 2022.3.10f1
m_EditorVersionWithRevision: 2022.3.10f1 (ff3792e53c62)
EOF

echo "Test project created at: $TEST_DIR"
echo ""
echo "You can now test the CLI with:"
echo "  ./dist/uniforge editor list"
echo "  ./dist/uniforge build --project $TEST_DIR --target ios --output ./Build"
echo ""