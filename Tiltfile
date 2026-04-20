# Tiltfile - Payment Service Development

# Configuração do Docker Compose
docker_compose('./docker-compose.yml')

# API Gateway Service
docker_build(
    'payment-service/api-gateway',
    context='.',
    dockerfile='./deployments/docker/Dockerfile.api',
    live_update=[
        sync('./cmd/api-gateway', '/app/cmd/api-gateway'),
        sync('./internal/api-gateway', '/app/internal/api-gateway'),
        sync('./pkg', '/app/pkg'),
        run('go build -o /app/bin/api-gateway ./cmd/api-gateway', trigger=['./cmd/api-gateway', './internal/api-gateway', './pkg']),
    ]
)

# Payment Service
docker_build(
    'payment-service/payment',
    context='.',
    dockerfile='./deployments/docker/Dockerfile.payment',
    live_update=[
        sync('./cmd/payment-service', '/app/cmd/payment-service'),
        sync('./internal/payment', '/app/internal/payment'),
        sync('./pkg', '/app/pkg'),
        run('go build -o /app/bin/payment-service ./cmd/payment-service', trigger=['./cmd/payment-service', './internal/payment', './pkg']),
    ]
)

# Ledger Service
docker_build(
    'payment-service/ledger',
    context='.',
    dockerfile='./deployments/docker/Dockerfile.ledger',
    live_update=[
        sync('./cmd/ledger-service', '/app/cmd/ledger-service'),
        sync('./internal/ledger', '/app/internal/ledger'),
        sync('./pkg', '/app/pkg'),
        run('go build -o /app/bin/ledger-service ./cmd/ledger-service', trigger=['./cmd/ledger-service', './internal/ledger', './pkg']),
    ]
)

# Resources do Kubernetes (quando migrarmos para K8s)
# k8s_yaml(['deployments/k8s/API Gateway.yaml', 'deployments/k8s/payment.yaml', 'deployments/k8s/ledger.yaml'])

# Porta dos serviços para acessar localmente
k8s_resource('API Gateway', port_forwards='8080:8080')
k8s_resource('payment-service', port_forwards='9090:9090')
k8s_resource('ledger-service', port_forwards='9091:9091')

# Aguarda os serviços de infraestrutura estarem prontos antes de subir os serviços
k8s_resource('postgres-payment', labels=['infra'])
k8s_resource('postgres-ledger', labels=['infra'])
k8s_resource('rabbitmq', labels=['infra'])
k8s_resource('redis', labels=['infra'])
k8s_resource('jaeger', labels=['infra'])

# Mensagens de status
print("""
┌───────────────────────────────────────────────────┐
│  Payment Service - Development Environment        │
├───────────────────────────────────────────────────┤
│  Services:                                        │
│    • API Gateway:             http://localhost:8080       │
│    • Payment Service: http://localhost:9090       │
│    • Ledger Service:  http://localhost:9091       │
│                                                   │
│  Infrastructure:                                  │
│    • RabbitMQ UI:     http://localhost:15672      │
│    • Jaeger UI:       http://localhost:16686      │
│    • PostgreSQL:      localhost:5432 / 5433       │
│    • Redis:           localhost:6379              │
└───────────────────────────────────────────────────┘
""")
