load('ext://configmap', 'configmap_create')

# ── Docker Builds ────────────────────────────────

docker_build(
    'payment-service-image',
    context='.',
    dockerfile='./deployments/docker/Dockerfile.payment',
    live_update=[
        sync('./cmd/payment-service', '/app/cmd/payment-service'),
        sync('./internal/payment', '/app/internal/payment'),
        sync('./pkg', '/app/pkg'),
        run('cd /app && go build -o /app/bin/payment-service ./cmd/payment-service',
            trigger=['./cmd/payment-service', './internal/payment', './pkg']),
    ],
)

docker_build(
    'ledger-service-image',
    context='.',
    dockerfile='./deployments/docker/Dockerfile.ledger',
    live_update=[
        sync('./cmd/ledger-service', '/app/cmd/ledger-service'),
        sync('./internal/ledger', '/app/internal/ledger'),
        sync('./pkg', '/app/pkg'),
        run('cd /app && go build -o /app/bin/ledger-service ./cmd/ledger-service',
            trigger=['./cmd/ledger-service', './internal/ledger', './pkg']),
    ],
)

docker_build(
    'api-gateway-image',
    context='.',
    dockerfile='./deployments/docker/Dockerfile.api',
    live_update=[
        sync('./cmd/api-gateway', '/app/cmd/api-gateway'),
        sync('./internal/api-gateway', '/app/internal/api-gateway'),
        sync('./pkg', '/app/pkg'),
        run('cd /app && go build -o /app/bin/api-gateway ./cmd/api-gateway',
            trigger=['./cmd/api-gateway', './internal/api-gateway', './pkg']),
    ],
)

# ── Migrations as ConfigMaps ────────────────────

configmap_create(
    'postgres-payment-initdb',
    from_file=[
        '000001_init_schema.up.sql=./deployments/migrations/payment/000001_init_schema.up.sql',
    ],
)

configmap_create(
    'postgres-ledger-initdb',
    from_file=[
        '000001_create_ledger_entries.up.sql=./deployments/migrations/ledger/000001_create_ledger_entries.up.sql',
        '000002_create_balances.up.sql=./deployments/migrations/ledger/000002_create_balances.up.sql',
    ],
)

# ── K8s Manifests ────────────────────────────────

k8s_yaml([
    './deployments/k8s/secrets.yaml',
    './deployments/k8s/config.yaml',
    './deployments/k8s/postgres-payment.yaml',
    './deployments/k8s/postgres-ledger.yaml',
    './deployments/k8s/rabbitmq.yaml',
    './deployments/k8s/payment-service.yaml',
    './deployments/k8s/ledger-service.yaml',
    './deployments/k8s/api-gateway.yaml',
])

# ── Resources & Port Forwards ───────────────────

k8s_resource('postgres-payment', labels=['infra'], port_forwards='5432:5432')
k8s_resource('postgres-ledger', labels=['infra'], port_forwards='5433:5432')
k8s_resource('rabbitmq', labels=['infra'], port_forwards=['5672:5672', '15672:15672'])

k8s_resource('payment-service', labels=['app'],
    resource_deps=['postgres-payment', 'rabbitmq'],
    port_forwards='9090:9090')

k8s_resource('ledger-service', labels=['app'],
    resource_deps=['postgres-ledger', 'rabbitmq'])

k8s_resource('api-gateway', labels=['app'],
    resource_deps=['payment-service'],
    port_forwards='8080:8080')

# ── Status ───────────────────────────────────────

print("""
┌─────────────────────────────────────────────────┐
│  Payment Service - K8s Dev Environment          │
├─────────────────────────────────────────────────┤
│  Services:                                      │
│    • API Gateway:       http://localhost:8080    │
│    • Payment Service:   localhost:9090 (gRPC)    │
│                                                 │
│  Infrastructure:                                │
│    • RabbitMQ UI:       http://localhost:15672   │
│    • PostgreSQL Payment: localhost:5432          │
│    • PostgreSQL Ledger:  localhost:5433          │
└─────────────────────────────────────────────────┘
""")
