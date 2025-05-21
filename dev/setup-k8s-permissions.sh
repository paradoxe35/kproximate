#!/bin/bash

# Script to set up Kubernetes permissions for local development
# This creates the necessary ServiceAccount, ClusterRole, and ClusterRoleBinding

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
NAMESPACE="kproximate-dev"
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

# Check if namespace exists, create it if it doesn't
if ! kubectl get namespace "$NAMESPACE" &> /dev/null; then
    print_info "Creating namespace $NAMESPACE"
    kubectl create namespace "$NAMESPACE"
fi

# Create a temporary directory for manifests
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

# Create ServiceAccount manifest
cat > "$TEMP_DIR/serviceaccount.yaml" << EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: $NAME
  namespace: $NAMESPACE
EOF

# Create ClusterRole manifest
cat > "$TEMP_DIR/clusterrole.yaml" << EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: $NAME
rules:
- apiGroups: [""]
  resources: ["pods/eviction"]
  verbs: ["create"]
# Needed to list pods by Node
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list"]
# Needed to cordon Nodes
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["get", "patch", "list", "update", "delete"]
# Needed to determine Pod owners
- apiGroups: ["apps"]
  resources: ["statefulsets"]
  verbs: ["get", "list"]
# Needed to determine Pod owners
- apiGroups: ["extensions"]
  resources: ["daemonsets", "replicasets"]
  verbs: ["get", "list"]
- apiGroups: ["storage.k8s.io"]
  resources: ["volumeattachments"]
  verbs: ["list", "delete"]
EOF

# Create ClusterRoleBinding manifest
cat > "$TEMP_DIR/clusterrolebinding.yaml" << EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: $NAME
subjects:
- kind: ServiceAccount
  name: $NAME
  namespace: $NAMESPACE
roleRef:
  kind: ClusterRole
  name: $NAME
  apiGroup: rbac.authorization.k8s.io
EOF

# Apply the manifests
print_info "Creating ServiceAccount $NAME in namespace $NAMESPACE"
kubectl apply -f "$TEMP_DIR/serviceaccount.yaml"

print_info "Creating ClusterRole $NAME"
kubectl apply -f "$TEMP_DIR/clusterrole.yaml"

print_info "Creating ClusterRoleBinding $NAME"
kubectl apply -f "$TEMP_DIR/clusterrolebinding.yaml"

# Create kubeconfig for the ServiceAccount
print_info "Creating kubeconfig for ServiceAccount $NAME"

# Get the ServiceAccount token
SECRET_NAME=$(kubectl get serviceaccount "$NAME" -n "$NAMESPACE" -o jsonpath='{.secrets[0].name}')
if [ -z "$SECRET_NAME" ]; then
    # For Kubernetes 1.24+, create a token
    print_info "Creating token for ServiceAccount $NAME"
    kubectl apply -f - << EOF
apiVersion: v1
kind: Secret
metadata:
  name: $NAME-token
  namespace: $NAMESPACE
  annotations:
    kubernetes.io/service-account.name: $NAME
type: kubernetes.io/service-account-token
EOF
    SECRET_NAME="$NAME-token"
    # Wait for the token to be populated
    print_info "Waiting for token to be populated..."
    sleep 3
fi

# Get the token
TOKEN=$(kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" -o jsonpath='{.data.token}' | base64 --decode)
if [ -z "$TOKEN" ]; then
    print_warning "Token not found in secret. For Kubernetes 1.24+, creating a temporary token."
    TOKEN=$(kubectl create token "$NAME" -n "$NAMESPACE")
fi

# Get cluster info
CLUSTER_NAME="kproximate-dev"
SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
CA_CERT=$(kubectl config view --minify --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')

# Create kubeconfig
KUBECONFIG_FILE="$TEMP_DIR/kubeconfig"
cat > "$KUBECONFIG_FILE" << EOF
apiVersion: v1
kind: Config
clusters:
- name: $CLUSTER_NAME
  cluster:
    server: $SERVER
    certificate-authority-data: $CA_CERT
contexts:
- name: $CLUSTER_NAME
  context:
    cluster: $CLUSTER_NAME
    user: $NAME
    namespace: $NAMESPACE
current-context: $CLUSTER_NAME
users:
- name: $NAME
  user:
    token: $TOKEN
EOF

# Save kubeconfig to dev directory
KUBECONFIG_PATH="$(pwd)/dev/kubeconfig"
cp "$KUBECONFIG_FILE" "$KUBECONFIG_PATH"
chmod 600 "$KUBECONFIG_PATH"

print_info "Kubernetes permissions set up successfully!"
print_info "ServiceAccount: $NAME"
print_info "Namespace: $NAMESPACE"
print_info "Kubeconfig saved to: $KUBECONFIG_PATH"
print_info ""
print_info "To use this kubeconfig with the development script, add the following to your .env.dev file:"
print_info "KUBECONFIG=$KUBECONFIG_PATH"
