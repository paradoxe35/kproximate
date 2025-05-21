# Kproximate Development Guide

This guide explains how to run Kproximate locally in development mode without deploying it with Helm.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Go](https://golang.org/doc/install) (version 1.22 or later)
- A Proxmox environment (for full functionality)

## Running Locally

The `dev.sh` script sets up a local development environment for Kproximate. It:

1. Starts a RabbitMQ container
2. Builds the controller and worker components
3. Runs both components in separate terminals

### Basic Usage

```bash
# From the project root
./dev/dev.sh
```

This will start Kproximate with default configuration values.

### Environment Configuration

The script supports loading configuration from a `.env.dev` file. To use this feature:

1. Copy the example environment file:
   ```bash
   cp dev/.env.example dev/.env.dev
   ```

2. Edit `dev/.env.dev` with your configuration values:
   ```bash
   # Edit the file with your preferred editor
   nano dev/.env.dev
   ```

3. Run the development script:
   ```bash
   ./dev/dev.sh
   ```

The `.env.dev` file is ignored by Git, so you can safely store sensitive information like Proxmox credentials.

### Command-Line Options

You can also provide configuration via command-line arguments, which will override values from the `.env.dev` file:

```bash
./dev/dev.sh --debug true --poll-interval 15 --max-kp-nodes 10
```

#### Available Options

| Option | Description | Default |
|--------|-------------|---------|
| `--debug` | Enable debug mode | `true` |
| `--poll-interval` | Poll interval in seconds | `10` |
| `--max-kp-nodes` | Maximum number of kproximate nodes | `5` |
| `--load-headroom` | Load headroom | `0.2` |
| `--wait-seconds-for-join` | Wait seconds for join | `60` |
| `--wait-seconds-for-provision` | Wait seconds for provision | `60` |
| `--pm-url` | Proxmox URL | - |
| `--pm-user-id` | Proxmox user ID | - |
| `--pm-password` | Proxmox password | - |
| `--pm-token` | Proxmox token | - |
| `--kp-node-template-name` | Kproximate node template name | - |

### Kubernetes Permissions Setup

Kproximate requires specific Kubernetes permissions to function properly. To set up these permissions for local development:

1. Run the setup script:
   ```bash
   ./dev/setup-k8s-permissions.sh
   ```

   This script will:
   - Create a ServiceAccount in your Kubernetes cluster
   - Create a ClusterRole with the necessary permissions
   - Create a ClusterRoleBinding to bind the ServiceAccount to the ClusterRole
   - Generate a kubeconfig file for the ServiceAccount

2. Add the generated kubeconfig path to your `.env.dev` file:
   ```
   KUBECONFIG=/path/to/kubeconfig
   ```

### Customizing Kubernetes Setup

You can customize the Kubernetes setup by providing options to the setup script:

```bash
./dev/setup-k8s-permissions.sh --namespace my-namespace --name my-kproximate
```

Available options:
- `--namespace`: Kubernetes namespace to use (default: default)
- `--name`: Name prefix for resources (default: kproximate-dev)

## Proxmox Configuration

For full functionality, you need to provide Proxmox configuration either in the `.env.dev` file or via command-line arguments:

```bash
./dev/dev.sh \
  --pm-url "https://your-proxmox-server:8006/api2/json" \
  --pm-user-id "root@pam" \
  --pm-password "your-password" \
  --kp-node-template-name "your-template-name"
```

## RabbitMQ Management UI

The RabbitMQ Management UI is available at http://localhost:15672 with the following credentials:

- Username: `guest` (or the value specified in your `.env.dev` file)
- Password: `guest` (or the value specified in your `.env.dev` file)

## Stopping the Application

To stop the application:

1. Close the terminal windows running the controller and worker
2. Stop the RabbitMQ container:

```bash
docker stop kproximate-rabbitmq
```

To remove the RabbitMQ container:

```bash
docker rm kproximate-rabbitmq
```

## Troubleshooting

### RabbitMQ Connection Issues

If you encounter connection issues with RabbitMQ, try restarting the container:

```bash
docker restart kproximate-rabbitmq
```

### Building Errors

If you encounter errors during the build process, make sure all dependencies are installed:

```bash
go get ./...
```

### Terminal Issues

The script attempts to open new terminal windows for the controller and worker. If this doesn't work on your system, you can manually run the components:

```bash
# In one terminal
./bin/controller

# In another terminal
./bin/worker
```
