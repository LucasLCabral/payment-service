# Makefile para Payment Service

.PHONY: help proto protog build test mocks loadclients k6-payments k8s-scale k8s-scale-down docker-up docker-down docker-logs tidy deps db-reset

# Kubernetes (kubectl configure no contexto atual)
K8S_NAMESPACE ?= default
SCALE_REPLICAS ?= 2
K6_IMAGE ?= grafana/k6:latest

help: ## Mostra esta mensagem de ajuda
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

protog: ## Gera stubs Go em protog/ a partir de proto/**/*.proto
	protoc --proto_path=. \
		--go_out=. --go_opt=module=github.com/LucasLCabral/payment-service \
		--go-grpc_out=. --go-grpc_opt=module=github.com/LucasLCabral/payment-service \
		proto/common/common.proto \
		proto/payment/payment.proto

proto: protog

build: ## Compila todos os serviços
	go build -o bin/api-gateway ./cmd/api-gateway
	go build -o bin/payment-service ./cmd/payment-service
	go build -o bin/ledger-service ./cmd/ledger-service
	go build -o bin/loadclients ./client/loadclients

loadclients: ## Compila só o simulador de clientes (vários atores)
	go build -o bin/loadclients ./client/loadclients

k6-payments: ## Carga k6 em POST /payments (API_BASE, VUS, DURATION, PAYER_ID, PAYEE_ID; usa `k6` local ou Docker)
	@if command -v k6 >/dev/null 2>&1; then \
		API_BASE="$${API_BASE:-http://127.0.0.1:8080}" VUS="$${VUS:-3}" DURATION="$${DURATION:-8s}" \
			IDEM_ITERATIONS="$${IDEM_ITERATIONS:-5}" \
			k6 run k6/payments_create.js; \
	else \
		echo "k6 não encontrado no PATH; usando Docker ($(K6_IMAGE))"; \
		docker run --rm -i \
			-v "$$(pwd)/k6:/scripts:ro" -w /scripts \
			-e API_BASE="$${API_BASE:-http://host.docker.internal:8080}" \
			-e VUS="$${VUS:-3}" -e DURATION="$${DURATION:-8s}" \
			-e IDEM_ITERATIONS="$${IDEM_ITERATIONS:-5}" \
			-e PAYER_ID -e PAYEE_ID \
			--add-host=host.docker.internal:host-gateway \
			$(K6_IMAGE) run payments_create.js; \
	fi

k8s-scale: ## Escala api-gateway, payment-service e ledger-service (SCALE_REPLICAS=3 K8S_NAMESPACE=default)
	kubectl scale deployment/api-gateway deployment/payment-service deployment/ledger-service \
		-n $(K8S_NAMESPACE) --replicas=$(SCALE_REPLICAS)

k8s-scale-down: ## Volta os três deployments para 1 réplica
	$(MAKE) k8s-scale SCALE_REPLICAS=1

test: ## Executa os testes
	go test -v -race ./...

mocks: ## Gera mocks (go generate em pacotes com //go:generate)
	go generate ./...

tidy: ## Limpa e organiza dependências
	go mod tidy

deps: ## Instala dependências do projeto
	go mod download

docker-up: ## Sobe a infraestrutura local (Docker Compose)
	docker-compose up -d

docker-down: ## Para a infraestrutura local
	docker-compose down

docker-logs: ## Mostra logs dos containers
	docker-compose logs -f

docker-clean: ## Remove volumes e limpa tudo
	docker-compose down -v

migrate-payment-up: ## Executa migrations do Payment Service
	@echo "Migrations serão executadas automaticamente pelo Tilt/Docker"

migrate-ledger-up: ## Executa migrations do Ledger Service
	@echo "Migrations serão executadas automaticamente pelo Tilt/Docker"

db-reset: ## Remove volumes dos Postgres e recria (initdb roda SQL em deployments/migrations/*)
	docker-compose down -v
	docker-compose up -d postgres-payment postgres-ledger
