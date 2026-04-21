# Makefile para Payment Service

.PHONY: help proto protog build test mocks docker-up docker-down docker-logs tidy deps db-reset

help: ## Mostra esta mensagem de ajuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

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
