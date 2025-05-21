#!/bin/bash

# Script to clean up Kubernetes permissions created by setup-k8s-permissions.sh
# This removes the ServiceAccount, ClusterRole, and ClusterRoleBinding

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Print with color
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if kubectl is installed
if ! command -v kubectl &> /dev/null; then
    print_error "kubectl is not installed. Please install kubectl first."
    exit 1
fi

# Check if kubectl can connect to a cluster
if ! kubectl cluster-info &> /dev/null; then
    print_error "kubectl cannot connect to a cluster. Please check your kubeconfig."
    exit 1
fi

# Default values
NAMESPACE="default"
NAME="kproximate-dev"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    key="$1"
    case $key in
        --namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        --name)
            NAME="$2"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [options]"
            echo "Options:"
            echo "  --namespace <namespace>  Kubernetes namespace to use (default: default)"
            echo "  --name <name>            Name prefix for resources (default: kproximate-dev)"
            echo "  --help                   Show this help message"
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Remove the ClusterRoleBinding
if kubectl get clusterrolebinding "$NAME" &> /dev/null; then
    print_info "Removing ClusterRoleBinding $NAME"
    kubectl delete clusterrolebinding "$NAME"
else
    print_warning "ClusterRoleBinding $NAME not found, skipping"
fi

# Remove the ClusterRole
if kubectl get clusterrole "$NAME" &> /dev/null; then
    print_info "Removing ClusterRole $NAME"
    kubectl delete clusterrole "$NAME"
else
    print_warning "ClusterRole $NAME not found, skipping"
fi

# Remove the ServiceAccount
if kubectl get serviceaccount -n "$NAMESPACE" "$NAME" &> /dev/null; then
    print_info "Removing ServiceAccount $NAME in namespace $NAMESPACE"
    kubectl delete serviceaccount -n "$NAMESPACE" "$NAME"
else
    print_warning "ServiceAccount $NAME in namespace $NAMESPACE not found, skipping"
fi

# Remove the token Secret if it exists
if kubectl get secret -n "$NAMESPACE" "$NAME-token" &> /dev/null; then
    print_info "Removing Secret $NAME-token in namespace $NAMESPACE"
    kubectl delete secret -n "$NAMESPACE" "$NAME-token"
else
    print_warning "Secret $NAME-token in namespace $NAMESPACE not found, skipping"
fi

# Remove the kubeconfig file
KUBECONFIG_PATH="$(pwd)/dev/kubeconfig"
if [ -f "$KUBECONFIG_PATH" ]; then
    print_info "Removing kubeconfig file $KUBECONFIG_PATH"
    rm "$KUBECONFIG_PATH"
else
    print_warning "Kubeconfig file $KUBECONFIG_PATH not found, skipping"
fi

print_info "Kubernetes permissions cleanup completed!"
