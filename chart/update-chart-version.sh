#!/bin/bash

# Script to update chart versions locally for testing
# Usage: ./update-chart-version.sh <version>

set -e

if [ $# -eq 0 ]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 1.2.3"
    exit 1
fi

VERSION=$1
CHART_DIR="$(dirname "$0")/kproximate"

# Validate version format (basic semver check)
if ! [[ $VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: Version must be in semver format (e.g., 1.2.3)"
    exit 1
fi

echo "Updating chart versions to $VERSION..."

# Check if yq is available
if ! command -v yq &> /dev/null; then
    echo "Error: yq is required but not installed."
    echo "Install it with: sudo wget -qO /usr/local/bin/yq https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 && sudo chmod +x /usr/local/bin/yq"
    exit 1
fi

# Update Chart.yaml
yq eval ".version = \"$VERSION\"" -i "$CHART_DIR/Chart.yaml"
yq eval ".appVersion = \"$VERSION\"" -i "$CHART_DIR/Chart.yaml"

echo "Updated Chart.yaml:"
echo "  version: $(yq eval '.version' "$CHART_DIR/Chart.yaml")"
echo "  appVersion: $(yq eval '.appVersion' "$CHART_DIR/Chart.yaml")"

echo "Chart versions updated successfully!"
echo ""
echo "To test the chart locally:"
echo "  helm dependency build $CHART_DIR"
echo "  helm template kproximate $CHART_DIR"
echo ""
echo "To package the chart:"
echo "  helm package $CHART_DIR --version $VERSION"
