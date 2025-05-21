# Makefile for Kproximate development

# Default values
NAMESPACE ?= kproximate-dev
NAME ?= kproximate-dev

.PHONY: help
help: ## Display this help message
	@echo "Kproximate Development Commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: dev-setup
dev-setup: ## Create .env.dev file from template if it doesn't exist
	@if [ ! -f dev/.env.dev ]; then \
		echo "Creating dev/.env.dev from template..."; \
		cp dev/.env.example dev/.env.dev; \
		echo "Created dev/.env.dev. Please edit it with your configuration."; \
	else \
		echo "dev/.env.dev already exists."; \
	fi

.PHONY: dev-run
dev-run: ## Run Kproximate in development mode
	@echo "Starting Kproximate in development mode..."
	@./dev/dev.sh

.PHONY: dev-k8s-setup
dev-k8s-setup: ## Set up Kubernetes permissions for development
	@echo "Setting up Kubernetes permissions..."
	@./dev/setup-k8s-permissions.sh --namespace $(NAMESPACE) --name $(NAME)

.PHONY: dev-k8s-cleanup
dev-k8s-cleanup: ## Clean up Kubernetes permissions
	@echo "Cleaning up Kubernetes permissions..."
	@./dev/cleanup-k8s-permissions.sh --namespace $(NAMESPACE) --name $(NAME)

.PHONY: dev-rabbitmq-start
dev-rabbitmq-start: ## Start RabbitMQ container for development
	@echo "Starting RabbitMQ container..."
	@docker run -d --name kproximate-rabbitmq \
		-p 5672:5672 \
		-p 15672:15672 \
		-e RABBITMQ_USERNAME=guest \
		-e RABBITMQ_PASSWORD=guest \
		-e RABBITMQ_SSL_VERIFY=verify_none \
		bitnami/rabbitmq:4.1 || \
	(docker start kproximate-rabbitmq && echo "RabbitMQ container already exists, starting it...")

.PHONY: dev-rabbitmq-stop
dev-rabbitmq-stop: ## Stop RabbitMQ container
	@echo "Stopping RabbitMQ container..."
	@docker stop kproximate-rabbitmq || echo "RabbitMQ container not running."

.PHONY: dev-rabbitmq-remove
dev-rabbitmq-remove: dev-rabbitmq-stop ## Remove RabbitMQ container
	@echo "Removing RabbitMQ container..."
	@docker rm kproximate-rabbitmq || echo "RabbitMQ container not found."

.PHONY: dev-deps
dev-deps: ## Install Go dependencies
	@echo "Installing Go dependencies..."
	@cd kproximate && go get ./...
	@echo "Dependencies installed."

.PHONY: dev-build
dev-build: dev-deps ## Build controller and worker binaries
	@echo "Building controller and worker..."
	@mkdir -p bin
	@cd kproximate && go build -o ../bin/controller controller/controller.go
	@cd kproximate && go build -o ../bin/worker worker/worker.go
	@echo "Binaries built in bin/ directory."

.PHONY: dev-clean
dev-clean: ## Remove built binaries
	@echo "Removing built binaries..."
	@rm -rf bin
	@echo "Binaries removed."

.PHONY: dev-test
dev-test: ## Run tests
	@echo "Running tests..."
	@cd kproximate && go test ./...

.PHONY: dev-all
dev-all: dev-setup dev-k8s-setup dev-build dev-run ## Set up everything and run in development mode

.PHONY: dev-stop-all
dev-stop-all: dev-rabbitmq-stop dev-k8s-cleanup ## Stop everything and clean up

.PHONY: dev-reset
dev-reset: dev-stop-all dev-clean ## Reset development environment completely
	@echo "Development environment reset."
