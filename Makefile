# Makefile para Payment Service

.PHONY: help proto build test docker-up docker-down docker-logs tidy deps

help: ## Mostra esta mensagem de ajuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

proto: ## Gera stubs gRPC a partir dos arquivos .proto
	protoc --go_out=. --go-grpc_out=. \
		proto/common/common.proto \
		proto/payment/payment.proto

build: ## Compila todos os serviços
	go build -o bin/API Gateway ./cmd/API Gateway
	go build -o bin/payment-service ./cmd/payment-service
	go build -o bin/ledger-service ./cmd/ledger-service

test: ## Executa os testes
	go test -v -race ./...

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
