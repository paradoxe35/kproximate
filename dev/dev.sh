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

# Default configuration values (these can be overridden by .env.dev)
# Using camelCase variable names that match what the application expects

# RabbitMQ defaults
rabbitMQContainerName="kproximate-rabbitmq"
rabbitMQUser="guest"
rabbitMQPassword="guest"
rabbitMQPort=5672
rabbitMQManagementPort=15672
rabbitMQUseTLS=false

# Application defaults
debug=true
pollInterval=10
maxKpNodes=5
loadHeadroom=0.2
waitSecondsForJoin=60
waitSecondsForProvision=60

# Scale-down stabilization defaults
scaleDownStabilizationMinutes=5
minNodeAgeMinutes=10

# Node selection strategy defaults
nodeSelectionStrategy="spread"
minAvailableCpuCores=0
minAvailableMemoryMB=0
excludedNodes=""

# Proxmox defaults
pmAllowInsecure=true
pmDebug=false
kpNodeNamePrefix="kp"
kpNodeCores=2
kpNodeMemory=2048
kpNodeDisableSsh=false
kpQemuExecJoin=true
kpLocalTemplateStorage=true

# Enhanced Autoscaling defaults
enableResourcePressureScaling=true
cpuUtilizationThreshold=0.8
memoryUtilizationThreshold=0.8
enableSchedulingErrorScaling=true
schedulingErrorThreshold=3
enableStoragePressureScaling=true
diskUtilizationThreshold=0.85
minAvailableDiskSpaceGB=5

# Load environment variables from .env.dev if it exists
ENV_FILE="$SCRIPT_DIR/.env.dev"
if [ -f "$ENV_FILE" ]; then
    print_info "Loading environment variables from $ENV_FILE"

    while IFS= read -r line || [[ -n "$line" ]]; do
        # Skip comments and empty lines
        if [[ "$line" =~ ^\s*#.*$ || -z "$line" ]]; then
            continue
        fi

        # Split the line into key and value
        key=$(echo "$line" | cut -d '=' -f 1)
        value=$(echo "$line" | cut -d '=' -f 2-)

        # Remove single quotes, double quotes, and leading/trailing spaces from the value
        value=$(echo "$value" | sed -e "s/^'//" -e "s/'$//" -e 's/^"//' -e 's/"$//' -e 's/^[ \t]*//;s/[ \t]*$//')

        # Export the key and value as environment variables
        export "$key=$value"

        print_info "Loaded $key"
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
            debug="$2"
            shift 2
            ;;
        --poll-interval)
            pollInterval="$2"
            shift 2
            ;;
        --max-kp-nodes)
            maxKpNodes="$2"
            shift 2
            ;;
        --load-headroom)
            loadHeadroom="$2"
            shift 2
            ;;
        --wait-seconds-for-join)
            waitSecondsForJoin="$2"
            shift 2
            ;;
        --wait-seconds-for-provision)
            waitSecondsForProvision="$2"
            shift 2
            ;;
        --pm-url)
            pmUrl="$2"
            shift 2
            ;;
        --pm-user-id)
            pmUserID="$2"
            shift 2
            ;;
        --pm-password)
            pmPassword="$2"
            shift 2
            ;;
        --pm-token)
            pmToken="$2"
            shift 2
            ;;
        --kp-node-template-name)
            kpNodeTemplateName="$2"
            shift 2
            ;;
        --node-selection-strategy)
            nodeSelectionStrategy="$2"
            shift 2
            ;;
        --min-available-cpu-cores)
            minAvailableCpuCores="$2"
            shift 2
            ;;
        --min-available-memory-mb)
            minAvailableMemoryMB="$2"
            shift 2
            ;;
        --excluded-nodes)
            excludedNodes="$2"
            shift 2
            ;;
        --scale-down-stabilization-minutes)
            scaleDownStabilizationMinutes="$2"
            shift 2
            ;;
        --min-node-age-minutes)
            minNodeAgeMinutes="$2"
            shift 2
            ;;
        --enable-resource-pressure-scaling)
            enableResourcePressureScaling="$2"
            shift 2
            ;;
        --cpu-utilization-threshold)
            cpuUtilizationThreshold="$2"
            shift 2
            ;;
        --memory-utilization-threshold)
            memoryUtilizationThreshold="$2"
            shift 2
            ;;
        --enable-scheduling-error-scaling)
            enableSchedulingErrorScaling="$2"
            shift 2
            ;;
        --scheduling-error-threshold)
            schedulingErrorThreshold="$2"
            shift 2
            ;;
        --enable-storage-pressure-scaling)
            enableStoragePressureScaling="$2"
            shift 2
            ;;
        --disk-utilization-threshold)
            diskUtilizationThreshold="$2"
            shift 2
            ;;
        --min-available-disk-space-gb)
            minAvailableDiskSpaceGB="$2"
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
            echo "  --node-selection-strategy <strategy> Node selection strategy (default: spread)"
            echo "                                       Options: spread, max-memory, max-cpu, balanced, round-robin"
            echo "  --min-available-cpu-cores <number>   Minimum available CPU cores required (default: 0)"
            echo "  --min-available-memory-mb <number>   Minimum available memory in MB required (default: 0)"
            echo "  --excluded-nodes <nodes>             Comma-separated list of nodes to exclude (default: \"\")"
            echo "  --scale-down-stabilization-minutes <number> Scale-down stabilization period in minutes (default: 5)"
            echo "  --min-node-age-minutes <number>     Minimum node age before scale-down in minutes (default: 10)"
            echo ""
            echo "Enhanced Autoscaling Options:"
            echo "  --enable-resource-pressure-scaling <true|false>  Enable CPU/memory pressure scaling (default: true)"
            echo "  --cpu-utilization-threshold <0.0-1.0>           CPU utilization threshold (default: 0.8)"
            echo "  --memory-utilization-threshold <0.0-1.0>        Memory utilization threshold (default: 0.8)"
            echo "  --enable-scheduling-error-scaling <true|false>  Enable scheduling error scaling (default: true)"
            echo "  --scheduling-error-threshold <number>           Number of failed pods threshold (default: 3)"
            echo "  --enable-storage-pressure-scaling <true|false>  Enable storage pressure scaling (default: true)"
            echo "  --disk-utilization-threshold <0.0-1.0>          Disk utilization threshold (default: 0.85)"
            echo "  --min-available-disk-space-gb <number>          Minimum available disk space in GB (default: 5)"
            echo ""
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
if [ -z "${pmUrl}" ] || [ -z "${pmUserID}" ] || [ -z "${kpNodeTemplateName}" ]; then
    print_warning "Proxmox configuration is incomplete. The application will start but may not function correctly."
    print_warning "Please provide pmUrl, pmUserID, and kpNodeTemplateName parameters."
    print_warning "Either pmPassword or pmToken is also required."
fi

# Start RabbitMQ container if it's not already running
if ! docker ps -a --format '{{.Names}}' | grep -q "^${rabbitMQContainerName}$"; then
    print_info "Starting RabbitMQ container..."
    docker run -d --name $rabbitMQContainerName \
        -p $rabbitMQPort:5672 \
        -p $rabbitMQManagementPort:15672 \
        -e RABBITMQ_USERNAME=$rabbitMQUser \
        -e RABBITMQ_PASSWORD=$rabbitMQPassword \
        rabbitmq:3-management

    # Wait for RabbitMQ to start
    print_info "Waiting for RabbitMQ to start..."
    sleep 10
else
    # Check if the container is running
    if ! docker ps --format '{{.Names}}' | grep -q "^${rabbitMQContainerName}$"; then
        print_info "Starting existing RabbitMQ container..."
        docker start $rabbitMQContainerName

        # Wait for RabbitMQ to start
        print_info "Waiting for RabbitMQ to start..."
        sleep 10
    else
        print_info "RabbitMQ container is already running."
    fi
fi

# Export environment variables for the application
# Since we're now using camelCase variables that match what the app expects,
# we can export them directly without conversion

# Core application configuration
export debug
export pollInterval
export maxKpNodes
export loadHeadroom
export waitSecondsForJoin
export waitSecondsForProvision

# RabbitMQ configuration
export rabbitMQHost="localhost"
export rabbitMQPort
export rabbitMQUser
export rabbitMQPassword
export rabbitMQUseTLS

# Kubernetes configuration
if [ ! -z "$KUBECONFIG" ]; then
    print_info "Using KUBECONFIG: $KUBECONFIG"
    export KUBECONFIG
fi

# Proxmox configuration
if [ ! -z "$pmUrl" ]; then
    export pmUrl
fi

if [ ! -z "$pmUserID" ]; then
    export pmUserID
fi

if [ ! -z "$pmPassword" ]; then
    export pmPassword
fi

if [ ! -z "$pmToken" ]; then
    export pmToken
fi

if [ ! -z "$kpNodeTemplateName" ]; then
    export kpNodeTemplateName
fi

# Additional Proxmox configuration
export pmAllowInsecure
export pmDebug
export kpNodeNamePrefix
export kpNodeCores
export kpNodeMemory
export kpNodeDisableSsh
export kpQemuExecJoin
export kpLocalTemplateStorage

# If sshKey is provided, export it
if [ ! -z "$sshKey" ]; then
    export sshKey
fi

# If kpNodeLabels is provided, export it
if [ ! -z "$kpNodeLabels" ]; then
    export kpNodeLabels
fi

# If kpJoinCommand is provided, export it
if [ ! -z "$kpJoinCommand" ]; then
    export kpJoinCommand
    print_info "Using kpJoinCommand from environment"
else
    print_warning "kpJoinCommand is not set. Nodes will not be able to join the Kubernetes cluster."
    print_warning "Set kpJoinCommand in your .env.dev file."
fi

# Node selection strategy configuration
export nodeSelectionStrategy
export minAvailableCpuCores
export minAvailableMemoryMB
export excludedNodes

# Scale-down stabilization configuration
export scaleDownStabilizationMinutes
export minNodeAgeMinutes

# Enhanced Autoscaling configuration
export enableResourcePressureScaling
export cpuUtilizationThreshold
export memoryUtilizationThreshold
export enableSchedulingErrorScaling
export schedulingErrorThreshold
export enableStoragePressureScaling
export diskUtilizationThreshold
export minAvailableDiskSpaceGB

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
    local kubeconfig_flag=""

    # Add kubeconfig flag if KUBECONFIG is set
    if [ ! -z "$KUBECONFIG" ]; then
        kubeconfig_flag="--kubeconfig=$KUBECONFIG"
        print_info "Using kubeconfig flag for $component: $kubeconfig_flag"
    fi

    # Use different terminal commands based on the OS
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        if command -v gnome-terminal &> /dev/null; then
            gnome-terminal --title="$title" -- bash -c "cd $(pwd) && ./bin/$component $kubeconfig_flag; exec bash"
        elif command -v xterm &> /dev/null; then
            xterm -title "$title" -e "cd $(pwd) && ./bin/$component $kubeconfig_flag; exec bash" &
        else
            print_warning "No suitable terminal emulator found. Running $component in the background."
            ./bin/$component $kubeconfig_flag &
        fi
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        osascript -e "tell app \"Terminal\" to do script \"cd $(pwd) && ./bin/$component $kubeconfig_flag\""
    else
        print_warning "Unsupported OS. Running $component in the background."
        ./bin/$component $kubeconfig_flag &
    fi
}

# Run the controller and worker
print_info "Starting controller and worker..."
run_component "controller" "Kproximate Controller"
run_component "worker" "Kproximate Worker"

print_info "Kproximate is running in development mode."
print_info "RabbitMQ Management UI: http://localhost:$rabbitMQManagementPort"
print_info "Username: $rabbitMQUser"
print_info "Password: $rabbitMQPassword"
print_info "Press Ctrl+C to stop the script (this will not stop the components)."

# Keep the script running
trap "print_info 'Exiting...'" SIGINT SIGTERM
while true; do
    sleep 1
done
