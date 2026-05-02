# Makefile para Payment Service

.PHONY: help proto protog build test test-all test-coverage mocks loadclients k6-payments k6-all k6-e2e k6-realistic k6-websocket k6-stress k6-chaos k8s-scale k8s-scale-down docker-up docker-down docker-logs tidy deps db-reset tilt tilt-down minikube-start minikube-stop

# Kubernetes (kubectl configure in the current context)
K8S_NAMESPACE ?= default
SCALE_REPLICAS ?= 2
K6_IMAGE ?= grafana/k6:latest

help: ## Show this help message
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

protog: ## Generate Go stubs in protog/ from proto/**/*.proto
	protoc --proto_path=. \
		--go_out=. --go_opt=module=github.com/LucasLCabral/payment-service \
		--go-grpc_out=. --go-grpc_opt=module=github.com/LucasLCabral/payment-service \
		proto/common/common.proto \
		proto/payment/payment.proto

proto: protog

build: ## Compile all services
	go build -o bin/api-gateway ./cmd/api-gateway
	go build -o bin/payment-service ./cmd/payment-service
	go build -o bin/ledger-service ./cmd/ledger-service

test: ## Run unit tests (fast)
	go test ./... -short -v

test-all: ## Run all tests including race detection
	go test -v -race ./...

# Integration tests removed - focus on unit tests + E2E with k6

test-coverage: ## Run tests with coverage report
	go test ./... -short -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Removed complex integration test setup - use existing docker-compose targets for manual testing

test-settlement: ## Run settlement handler tests specifically
	go test ./internal/payment/settlement/... -v

test-repository: ## Run repository tests specifically  
	go test ./internal/payment/repository/... -v

test-bench: ## Run benchmarks
	go test ./... -bench=. -benchmem -short

loadclients: ## Compile only the client simulator (multiple actors)
	go build -o bin/loadclients ./client/loadclients

k6-payments: ## Load k6 in POST /payments (API_BASE, VUS, DURATION, PAYER_ID, PAYEE_ID; use `k6` local or Docker)
	@if command -v k6 >/dev/null 2>&1; then \
		API_BASE="$${API_BASE:-http://127.0.0.1:8080}" VUS="$${VUS:-3}" DURATION="$${DURATION:-8s}" \
			IDEM_ITERATIONS="$${IDEM_ITERATIONS:-5}" \
			PAYER_ID="$${PAYER_ID:-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}" \
			PAYEE_ID="$${PAYEE_ID:-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}" \
			k6 run k6/payments_create.js; \
	else \
		echo "k6 not found in PATH; using Docker ($(K6_IMAGE))"; \
		docker run --rm -i \
			-v "$$(pwd)/k6:/scripts:ro" -w /scripts \
			-e API_BASE="$${API_BASE:-http://host.docker.internal:8080}" \
			-e VUS="$${VUS:-3}" -e DURATION="$${DURATION:-8s}" \
			-e IDEM_ITERATIONS="$${IDEM_ITERATIONS:-5}" \
			-e PAYER_ID="$${PAYER_ID:-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}" \
			-e PAYEE_ID="$${PAYEE_ID:-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}" \
			--add-host=host.docker.internal:host-gateway \
			$(K6_IMAGE) run payments_create.js; \
	fi

k6-websocket: ## WebSocket tests for real-time notifications
	@if command -v k6 >/dev/null 2>&1; then \
		API_BASE="$${API_BASE:-http://127.0.0.1:8080}" \
			WS_BASE="$${WS_BASE:-ws://127.0.0.1:8080}" \
			WS_VUS="$${WS_VUS:-2}" WS_DURATION="$${WS_DURATION:-30s}" \
			PAYER_ID="$${PAYER_ID:-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}" \
			PAYEE_ID="$${PAYEE_ID:-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}" \
			k6 run k6/websocket_notifications.js; \
	else \
		docker run --rm -i \
			-v "$$(pwd)/k6:/scripts:ro" -w /scripts \
			-e API_BASE="$${API_BASE:-http://host.docker.internal:8080}" \
			-e WS_BASE="$${WS_BASE:-ws://host.docker.internal:8080}" \
			-e WS_VUS="$${WS_VUS:-2}" -e WS_DURATION="$${WS_DURATION:-30s}" \
			-e PAYER_ID="$${PAYER_ID:-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}" \
			-e PAYEE_ID="$${PAYEE_ID:-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}" \
			--add-host=host.docker.internal:host-gateway \
			$(K6_IMAGE) run websocket_notifications.js; \
	fi

k6-e2e: ## End-to-end tests for the complete payment flow
	@if command -v k6 >/dev/null 2>&1; then \
		API_BASE="$${API_BASE:-http://127.0.0.1:8080}" \
			E2E_VUS="$${E2E_VUS:-3}" E2E_DURATION="$${E2E_DURATION:-60s}" \
			E2E_ITERATIONS="$${E2E_ITERATIONS:-10}" \
			PAYER_ID="$${PAYER_ID:-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}" \
			PAYEE_ID="$${PAYEE_ID:-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}" \
			k6 run k6/e2e_flow.js; \
	else \
		docker run --rm -i \
			-v "$$(pwd)/k6:/scripts:ro" -w /scripts \
			-e API_BASE="$${API_BASE:-http://host.docker.internal:8080}" \
			-e E2E_VUS="$${E2E_VUS:-3}" -e E2E_DURATION="$${E2E_DURATION:-60s}" \
			-e E2E_ITERATIONS="$${E2E_ITERATIONS:-10}" \
			-e PAYER_ID="$${PAYER_ID:-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}" \
			-e PAYEE_ID="$${PAYEE_ID:-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}" \
			--add-host=host.docker.internal:host-gateway \
			$(K6_IMAGE) run e2e_flow.js; \
	fi

k6-chaos: ## Chaos testing for failure scenarios and resilience
	@if command -v k6 >/dev/null 2>&1; then \
		API_BASE="$${API_BASE:-http://127.0.0.1:8080}" \
			CHAOS_TYPE="$${CHAOS_TYPE}" \
			PAYER_ID="$${PAYER_ID:-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}" \
			PAYEE_ID="$${PAYEE_ID:-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}" \
			k6 run k6/chaos_scenarios.js; \
	else \
		docker run --rm -i \
			-v "$$(pwd)/k6:/scripts:ro" -w /scripts \
			-e API_BASE="$${API_BASE:-http://host.docker.internal:8080}" \
			-e CHAOS_TYPE="$${CHAOS_TYPE}" \
			-e PAYER_ID="$${PAYER_ID:-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}" \
			-e PAYEE_ID="$${PAYEE_ID:-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}" \
			--add-host=host.docker.internal:host-gateway \
			$(K6_IMAGE) run chaos_scenarios.js; \
	fi

k6-realistic: ## Realistic load testing with different user profiles
	@if command -v k6 >/dev/null 2>&1; then \
		API_BASE="$${API_BASE:-http://127.0.0.1:8080}" \
			MOBILE_VUS="$${MOBILE_VUS:-8}" \
			PAYER_ID="$${PAYER_ID:-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}" \
			PAYEE_ID="$${PAYEE_ID:-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}" \
			k6 run k6/realistic_load_test.js; \
	else \
		docker run --rm -i \
			-v "$$(pwd)/k6:/scripts:ro" -w /scripts \
			-e API_BASE="$${API_BASE:-http://host.docker.internal:8080}" \
			-e MOBILE_VUS="$${MOBILE_VUS:-8}" \
			-e PAYER_ID="$${PAYER_ID:-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}" \
			-e PAYEE_ID="$${PAYEE_ID:-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}" \
			--add-host=host.docker.internal:host-gateway \
			$(K6_IMAGE) run realistic_load_test.js; \
	fi

k6-metrics: ## Advanced business metrics and observability collection
	@if command -v k6 >/dev/null 2>&1; then \
		API_BASE="$${API_BASE:-http://127.0.0.1:8080}" \
			METRICS_VUS="$${METRICS_VUS:-5}" METRICS_DURATION="$${METRICS_DURATION:-120s}" \
			PAYER_ID="$${PAYER_ID:-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}" \
			PAYEE_ID="$${PAYEE_ID:-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}" \
			k6 run k6/advanced_metrics_test.js; \
	else \
		docker run --rm -i \
			-v "$$(pwd)/k6:/scripts:ro" -w /scripts \
			-e API_BASE="$${API_BASE:-http://host.docker.internal:8080}" \
			-e METRICS_VUS="$${METRICS_VUS:-5}" -e METRICS_DURATION="$${METRICS_DURATION:-120s}" \
			-e PAYER_ID="$${PAYER_ID:-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}" \
			-e PAYEE_ID="$${PAYEE_ID:-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}" \
			--add-host=host.docker.internal:host-gateway \
			$(K6_IMAGE) run advanced_metrics_test.js; \
	fi

k6-stress: ## Stress testing - Progressive load until breaking point (WS_STRESS_VUS, API_BASE, WS_BASE)
	@if command -v k6 >/dev/null 2>&1; then \
		API_BASE="$${API_BASE:-http://127.0.0.1:8080}" \
			WS_BASE="$${WS_BASE:-ws://127.0.0.1:8080}" \
			WS_STRESS_VUS="$${WS_STRESS_VUS:-100}" \
			PAYER_ID="$${PAYER_ID:-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}" \
			PAYEE_ID="$${PAYEE_ID:-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}" \
			k6 run k6/stress_test.js; \
	else \
		docker run --rm -i \
			-v "$$(pwd)/k6:/scripts:ro" -w /scripts \
			-e API_BASE="$${API_BASE:-http://host.docker.internal:8080}" \
			-e WS_BASE="$${WS_BASE:-ws://host.docker.internal:8080}" \
			-e WS_STRESS_VUS="$${WS_STRESS_VUS:-100}" \
			-e PAYER_ID="$${PAYER_ID:-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}" \
			-e PAYEE_ID="$${PAYEE_ID:-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}" \
			--add-host=host.docker.internal:host-gateway \
			$(K6_IMAGE) run stress_test.js; \
	fi

k6-all: ## Run all k6 tests in sequence (e2e -> realistic -> websocket -> stress -> chaos)
	@echo "🚀 Running complete k6 test suite..."
	@$(MAKE) k6-e2e E2E_DURATION=30s
	@echo "✅ E2E tests completed"
	@sleep 2  
	@$(MAKE) k6-realistic MOBILE_VUS=4
	@echo "✅ Realistic load tests completed"
	@sleep 2
	@$(MAKE) k6-websocket
	@echo "✅ WebSocket tests completed"
	@sleep 2
	@$(MAKE) k6-stress
	@echo "✅ Stress tests completed"
	@sleep 2
	@$(MAKE) k6-chaos
	@echo "✅ Chaos tests completed"
	@echo "🎉 All k6 tests completed!"

k8s-scale: ## Scale api-gateway, payment-service and ledger-service (SCALE_REPLICAS=3 K8S_NAMESPACE=default)
	kubectl scale deployment/api-gateway deployment/payment-service deployment/ledger-service \
		-n $(K8S_NAMESPACE) --replicas=$(SCALE_REPLICAS)

k8s-scale-down: ## Scale down the three deployments to 1 replica
	$(MAKE) k8s-scale SCALE_REPLICAS=1

k8s-hpa-status: ## Show HPA status and current scaling metrics
	@echo "📊 Horizontal Pod Autoscaler Status"
	@echo "=================================="
	kubectl get hpa -n $(K8S_NAMESPACE)
	@echo ""
	@echo "📈 Detailed HPA Metrics:"
	@echo "----------------------"
	kubectl describe hpa api-gateway-hpa -n $(K8S_NAMESPACE) || echo "api-gateway-hpa not found"
	@echo ""
	kubectl describe hpa payment-service-hpa -n $(K8S_NAMESPACE) || echo "payment-service-hpa not found"
	@echo ""
	kubectl describe hpa ledger-service-hpa -n $(K8S_NAMESPACE) || echo "ledger-service-hpa not found"

k8s-pods-status: ## Show current pod replicas and resource usage
	@echo "🔍 Current Pod Status"
	@echo "==================="
	kubectl get pods -n $(K8S_NAMESPACE) -l app=api-gateway -o wide
	kubectl get pods -n $(K8S_NAMESPACE) -l app=payment-service -o wide
	kubectl get pods -n $(K8S_NAMESPACE) -l app=ledger-service -o wide
	@echo ""
	@echo "📊 Resource Usage:"
	@echo "=================="
	kubectl top pods -n $(K8S_NAMESPACE) --containers 2>/dev/null || echo "Metrics server not available"

k8s-watch-scaling: ## Watch pod scaling in real-time during stress test
	@echo "👀 Watching pod scaling (press Ctrl+C to stop)"
	@echo "=============================================="
	watch -n 2 'echo "🔄 $(shell date)"; kubectl get pods -n $(K8S_NAMESPACE) -l "app in (api-gateway,payment-service,ledger-service)" -o wide; echo ""; kubectl get hpa -n $(K8S_NAMESPACE); echo ""; kubectl top pods -n $(K8S_NAMESPACE) 2>/dev/null || echo "Metrics not available"'

k8s-stress-prepare: ## Prepare cluster for stress testing (enable HPA, check metrics server)
	@echo "🚀 Preparing cluster for stress testing..."
	@echo "========================================"
	@echo "✅ Checking if metrics-server is running..."
	kubectl get deployment metrics-server -n kube-system >/dev/null 2>&1 || echo "⚠️  Metrics server not found - install with: kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml"
	@echo "✅ Applying HPA configurations..."
	kubectl apply -f deployments/k8s/hpa.yaml -n $(K8S_NAMESPACE)
	@echo "✅ Current HPA status:"
	kubectl get hpa -n $(K8S_NAMESPACE)
	@echo ""
	@echo "🎯 Ready for stress testing! Run: make k6-stress"
	@echo "📊 Monitor scaling with: make k8s-watch-scaling"

k8s-stress-full: ## Complete stress test with automatic scaling monitoring and analysis
	@echo "🚀 Starting full stress test with auto-scaling monitoring..."
	./scripts/stress-test-with-monitoring.sh

# Duplicate target removed - see test-all above

mocks: ## Generate mocks (go generate in packages with //go:generate)
	go generate ./...

tidy: ## Clean and organize dependencies
	go mod tidy

deps: ## Install project dependencies
	go mod download

docker-up: ## Spin up the local infrastructure (Docker Compose)
	docker-compose up -d

docker-down: ## Bring down the local infrastructure
	docker-compose down

docker-logs: ## Show logs of the containers
	docker-compose logs -f

docker-clean: ## Remove volumes and clean everything
	docker-compose down -v

# ── Kubernetes Development with Minikube & Tilt ──

minikube-start: ## Start Minikube with recommended resources
	@echo "🚀 Starting Minikube with recommended resources..."
	minikube start --cpus=4 --memory=8192 --driver=docker
	@echo "✅ Minikube started successfully!"
	@echo "💡 Enable ingress if needed: minikube addons enable ingress"

minikube-stop: ## Stop Minikube
	@echo "🛑 Stopping Minikube..."
	minikube stop
	@echo "✅ Minikube stopped!"

minikube-status: ## Check Minikube status and cluster info
	@echo "📊 Minikube Status:"
	@minikube status
	@echo "\n🔗 Cluster Info:"
	@kubectl cluster-info
	@echo "\n📦 Nodes:"
	@kubectl get nodes

tilt: ## Start development environment with Tilt (requires Minikube)
	@echo "🚀 Starting Tilt development environment..."
	@echo "💡 Make sure Minikube is running: make minikube-start"
	@echo "🔄 Building and deploying services to Kubernetes..."
	tilt up

tilt-down: ## Stop Tilt and clean up resources
	@echo "🛑 Stopping Tilt and cleaning up resources..."
	tilt down
	@echo "✅ Tilt stopped and resources cleaned up!"

tilt-ci: ## Run Tilt in CI mode (non-interactive)
	tilt ci

migrate-payment-up: ## Run migrations for the Payment Service
	@echo "Migrations will be executed automatically by Tilt/Docker"

migrate-ledger-up: ## Run migrations for the Ledger Service
	@echo "Migrations will be executed automatically by Tilt/Docker"

db-reset: ## Remove volumes of the Postgres and recreate (initdb runs SQL in deployments/migrations/*)
	docker-compose down -v
	docker-compose up -d postgres-payment postgres-ledger

db-add-test-balances: ## Add money to test accounts (aaa..., bbb..., etc) for k6 tests (K8s)
	@echo "💰 Adding test balances to Kubernetes ledger database..."
	@echo "Using kubectl exec to connect to postgres-ledger-0..."
	kubectl exec -i postgres-ledger-0 -- psql -U ledger_user -d ledger_db < scripts/add-test-balances.sql
	@echo "✅ Test balances added successfully!"

db-check-balances: ## Show current balances of all test accounts (K8s)
	@echo "💳 Current account balances (Kubernetes):"
	@echo "========================================"
	kubectl exec -i postgres-ledger-0 -- psql -U ledger_user -d ledger_db -c "SELECT account_id, currency, ROUND(amount_cents / 100.0, 2) AS amount_reais, updated_at FROM balances ORDER BY amount_cents DESC;"
