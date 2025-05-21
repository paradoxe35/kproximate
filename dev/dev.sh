#!/bin/bash

# Script to run kproximate locally in development mode
# This script sets up the necessary dependencies and runs the controller and worker components

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

# Get the directory of the script
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." &> /dev/null && pwd )"

# Default configuration values
RABBITMQ_CONTAINER_NAME="kproximate-rabbitmq"
RABBITMQ_USER="guest"
RABBITMQ_PASSWORD="guest"
RABBITMQ_PORT=5672
RABBITMQ_MANAGEMENT_PORT=15672

# Default kproximate configuration
DEBUG=true
POLL_INTERVAL=10
MAX_KP_NODES=5
LOAD_HEADROOM=0.2
WAIT_SECONDS_FOR_JOIN=60
WAIT_SECONDS_FOR_PROVISION=60

# Load environment variables from .env.dev if it exists
ENV_FILE="$SCRIPT_DIR/.env.dev"
if [ -f "$ENV_FILE" ]; then
    print_info "Loading environment variables from $ENV_FILE"

    # Read .env.dev file and export variables
    while IFS= read -r line || [[ -n "$line" ]]; do
        # Skip comments and empty lines
        if [[ $line =~ ^#.*$ ]] || [[ -z $line ]]; then
            continue
        fi

        # Export the variable
        export "$line"

        # Extract variable name for logging
        var_name=$(echo "$line" | cut -d= -f1)
        print_info "Loaded $var_name"
    done < "$ENV_FILE"
else
    print_warning "$ENV_FILE not found. Using default values."
    print_info "You can create this file based on $SCRIPT_DIR/.env.example"
fi

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    print_error "Docker is not installed. Please install Docker first."
    exit 1
fi

# Check if Go is installed
if ! command -v go &> /dev/null; then
    print_error "Go is not installed. Please install Go first."
    exit 1
fi

# Parse command line arguments (these override .env.dev values)
while [[ $# -gt 0 ]]; do
    key="$1"
    case $key in
        --debug)
            DEBUG="$2"
            shift 2
            ;;
        --poll-interval)
            POLL_INTERVAL="$2"
            shift 2
            ;;
        --max-kp-nodes)
            MAX_KP_NODES="$2"
            shift 2
            ;;
        --load-headroom)
            LOAD_HEADROOM="$2"
            shift 2
            ;;
        --wait-seconds-for-join)
            WAIT_SECONDS_FOR_JOIN="$2"
            shift 2
            ;;
        --wait-seconds-for-provision)
            WAIT_SECONDS_FOR_PROVISION="$2"
            shift 2
            ;;
        --pm-url)
            PM_URL="$2"
            shift 2
            ;;
        --pm-user-id)
            PM_USER_ID="$2"
            shift 2
            ;;
        --pm-password)
            PM_PASSWORD="$2"
            shift 2
            ;;
        --pm-token)
            PM_TOKEN="$2"
            shift 2
            ;;
        --kp-node-template-name)
            KP_NODE_TEMPLATE_NAME="$2"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [options]"
            echo "Options:"
            echo "  --debug <true|false>                 Enable debug mode (default: true)"
            echo "  --poll-interval <seconds>            Poll interval in seconds (default: 10)"
            echo "  --max-kp-nodes <number>              Maximum number of kproximate nodes (default: 5)"
            echo "  --load-headroom <float>              Load headroom (default: 0.2)"
            echo "  --wait-seconds-for-join <seconds>    Wait seconds for join (default: 60)"
            echo "  --wait-seconds-for-provision <seconds> Wait seconds for provision (default: 60)"
            echo "  --pm-url <url>                       Proxmox URL"
            echo "  --pm-user-id <user-id>               Proxmox user ID"
            echo "  --pm-password <password>             Proxmox password"
            echo "  --pm-token <token>                   Proxmox token"
            echo "  --kp-node-template-name <name>       Kproximate node template name"
            echo "  --help                               Show this help message"
            echo ""
            echo "Environment variables can also be set in $SCRIPT_DIR/.env.dev"
            echo "See $SCRIPT_DIR/.env.example for available variables"
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Check if required Proxmox parameters are provided
if [ -z "${PM_URL}" ] || [ -z "${PM_USER_ID}" ] || [ -z "${KP_NODE_TEMPLATE_NAME}" ]; then
    print_warning "Proxmox configuration is incomplete. The application will start but may not function correctly."
    print_warning "Please provide PM_URL, PM_USER_ID, and KP_NODE_TEMPLATE_NAME parameters."
    print_warning "Either PM_PASSWORD or PM_TOKEN is also required."
fi

# Start RabbitMQ container if it's not already running
if ! docker ps -a --format '{{.Names}}' | grep -q "^${RABBITMQ_CONTAINER_NAME}$"; then
    print_info "Starting RabbitMQ container..."
    docker run -d --name $RABBITMQ_CONTAINER_NAME \
        -p $RABBITMQ_PORT:5672 \
        -p $RABBITMQ_MANAGEMENT_PORT:15672 \
        -e RABBITMQ_DEFAULT_USER=$RABBITMQ_USER \
        -e RABBITMQ_DEFAULT_PASS=$RABBITMQ_PASSWORD \
        rabbitmq:3-management

    # Wait for RabbitMQ to start
    print_info "Waiting for RabbitMQ to start..."
    sleep 10
else
    # Check if the container is running
    if ! docker ps --format '{{.Names}}' | grep -q "^${RABBITMQ_CONTAINER_NAME}$"; then
        print_info "Starting existing RabbitMQ container..."
        docker start $RABBITMQ_CONTAINER_NAME

        # Wait for RabbitMQ to start
        print_info "Waiting for RabbitMQ to start..."
        sleep 10
    else
        print_info "RabbitMQ container is already running."
    fi
fi

# Set environment variables for the application
export debug=$DEBUG
export pollInterval=$POLL_INTERVAL
export maxKpNodes=$MAX_KP_NODES
export loadHeadroom=$LOAD_HEADROOM
export waitSecondsForJoin=$WAIT_SECONDS_FOR_JOIN
export waitSecondsForProvision=$WAIT_SECONDS_FOR_PROVISION

# RabbitMQ configuration
export rabbitMQHost="localhost"
export rabbitMQPort=$RABBITMQ_PORT
export rabbitMQUser=$RABBITMQ_USER
export rabbitMQPassword=$RABBITMQ_PASSWORD

# Kubernetes configuration
if [ ! -z "$KUBECONFIG" ]; then
    print_info "Using KUBECONFIG: $KUBECONFIG"
    export KUBECONFIG=$KUBECONFIG
fi

# Proxmox configuration
if [ ! -z "$PM_URL" ]; then
    export pmUrl=$PM_URL
fi

if [ ! -z "$PM_USER_ID" ]; then
    export pmUserID=$PM_USER_ID
fi

if [ ! -z "$PM_PASSWORD" ]; then
    export pmPassword=$PM_PASSWORD
fi

if [ ! -z "$PM_TOKEN" ]; then
    export pmToken=$PM_TOKEN
fi

if [ ! -z "$KP_NODE_TEMPLATE_NAME" ]; then
    export kpNodeTemplateName=$KP_NODE_TEMPLATE_NAME
fi

# Additional configuration with default values
export pmAllowInsecure=${PM_ALLOW_INSECURE:-true}
export pmDebug=${PM_DEBUG:-$DEBUG}
export kpNodeNamePrefix=${KP_NODE_NAME_PREFIX:-"kp"}
export kpNodeCores=${KP_NODE_CORES:-2}
export kpNodeMemory=${KP_NODE_MEMORY:-2048}
export kpNodeDisableSsh=${KP_NODE_DISABLE_SSH:-false}
export kpQemuExecJoin=${KP_QEMU_EXEC_JOIN:-true}
export kpLocalTemplateStorage=${KP_LOCAL_TEMPLATE_STORAGE:-true}

# If SSH_KEY is provided, export it
if [ ! -z "$SSH_KEY" ]; then
    export sshKey=$SSH_KEY
fi

# If KP_NODE_LABELS is provided, export it
if [ ! -z "$KP_NODE_LABELS" ]; then
    export kpNodeLabels=$KP_NODE_LABELS
fi

# If KP_JOIN_COMMAND is provided, export it
if [ ! -z "$KP_JOIN_COMMAND" ]; then
    export kpJoinCommand=$KP_JOIN_COMMAND
    print_info "Using kpJoinCommand from environment"
else
    print_warning "kpJoinCommand is not set. Nodes will not be able to join the Kubernetes cluster."
    print_warning "Set KP_JOIN_COMMAND in your .env.dev file."
fi

# Change to project root directory
cd $PROJECT_ROOT

# Create bin directory if it doesn't exist
mkdir -p bin

# Build the controller and worker
print_info "Building controller and worker..."
cd kproximate
go build -o ../bin/controller controller/controller.go
go build -o ../bin/worker worker/worker.go
cd ..

# Function to run a component in a new terminal
run_component() {
    local component=$1
    local title=$2

    # Use different terminal commands based on the OS
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        if command -v gnome-terminal &> /dev/null; then
            gnome-terminal --title="$title" -- bash -c "cd $(pwd) && ./bin/$component; exec bash"
        elif command -v xterm &> /dev/null; then
            xterm -title "$title" -e "cd $(pwd) && ./bin/$component; exec bash" &
        else
            print_warning "No suitable terminal emulator found. Running $component in the background."
            ./bin/$component &
        fi
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        osascript -e "tell app \"Terminal\" to do script \"cd $(pwd) && ./bin/$component\""
    else
        print_warning "Unsupported OS. Running $component in the background."
        ./bin/$component &
    fi
}

# Run the controller and worker
print_info "Starting controller and worker..."
run_component "controller" "Kproximate Controller"
run_component "worker" "Kproximate Worker"

print_info "Kproximate is running in development mode."
print_info "RabbitMQ Management UI: http://localhost:$RABBITMQ_MANAGEMENT_PORT"
print_info "Username: $RABBITMQ_USER"
print_info "Password: $RABBITMQ_PASSWORD"
print_info "Press Ctrl+C to stop the script (this will not stop the components)."

# Keep the script running
trap "print_info 'Exiting...'" SIGINT SIGTERM
while true; do
    sleep 1
done
