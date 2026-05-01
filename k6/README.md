# k6 Test Suite - Payment Service

This suite contains comprehensive performance tests for the payment system, covering everything from basic validations to complex load and resilience scenarios.

## 📋 Test Overview

### 1. **WebSocket Tests** (`websocket_notifications.js`)
- **Objective**: Validate real-time notifications
- **Scenarios**: WS connections, notification receipt, connection stress test
- **Metrics**: Notification latency, concurrent connections, error rate
- **Command**: `make k6-websocket`

### 2. **End-to-End Tests** (`e2e_flow.js`)
- **Objective**: Test complete payment flow
- **Scenarios**: Creation → Settlement → Validation, batch tests, SLA compliance
- **Metrics**: Total settlement time, success rate, polling efficiency
- **Command**: `make k6-e2e`

### 3. **Chaos Tests** (`chaos_scenarios.js`)
- **Objective**: Validate resilience under failures
- **Scenarios**: Slow DB, unstable RabbitMQ, high network latency, traffic spikes
- **Metrics**: Error rates during chaos, recovery time, resilience score
- **Command**: `make k6-chaos`

### 4. **Realistic Load Tests** (`realistic_load.js`)
- **Objective**: Simulate real usage patterns
- **Scenarios**: Retail users (PIX), corporate (TED), e-commerce, mobile, Black Friday
- **Metrics**: Behavior by user type, sessions, temporal distribution
- **Command**: `make k6-realistic`

### 5. **Advanced Metrics** (`advanced_metrics.js`)
- **Objective**: Detailed business metrics collection
- **Scenarios**: Business intelligence, compliance monitoring, SLI/SLO tracking
- **Metrics**: Financial volume, KYC compliance, user satisfaction, error budget
- **Command**: `make k6-metrics`

## 🚀 Quick Execution

```bash
# All tests in sequence
make k6-all

# Specific test
make k6-e2e
make k6-realistic
```

## ⚙️ Configuration

### Environment Variables

```bash
# Basic configuration
export API_BASE="http://localhost:8080"
export WS_BASE="ws://localhost:8080" 
export PAYER_ID="aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
export PAYEE_ID="bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

# Load configuration
export VUS=10                    # Virtual users for basic tests
export DURATION="60s"            # Test duration
export E2E_VUS=5                 # VUs for E2E tests
export WS_VUS=3                  # VUs for WebSocket tests
export MOBILE_VUS=8              # VUs for mobile simulation
export METRICS_VUS=5             # VUs for metrics collection

# Chaos configuration
export CHAOS_TYPE="db_latency"   # Chaos type: db_latency, network_latency, etc
```

### k6 Installation

```bash
# macOS
brew install k6

# Ubuntu/Debian  
sudo apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
echo "deb https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update
sudo apt-get install k6

# Or use Docker (automatic in Makefile)
docker run --rm -i grafana/k6:latest run - < script.js
```

## 📊 Metrics and Thresholds

### Monitored SLAs

| Metric | Threshold | Description |
|---------|-----------|-----------|
| `http_req_duration` | P95 < 2s | 95% of requests in < 2s |
| `http_req_failed` | < 15% | Maximum error rate |
| `settlement_latency_ms` | P90 < 10s | 90% settlements in < 10s |
| `websocket_notification_latency_ms` | P95 < 5s | Notifications in < 5s |
| `quality_availability_sli` | ≥ 99.9% | Availability SLA |

### Business Metrics

- **Financial Volume**: Total processed by category (PIX, TED, etc.)
- **Compliance**: KYC required payments, suspicious activity rate
- **User Experience**: Journey success rate, satisfaction score
- **Operational**: Throughput per endpoint, resource utilization

## 🎯 Scenarios by Environment

### Local Development
```bash
# Light tests for development
make k6-smoke
make k6-e2e E2E_DURATION=30s E2E_VUS=2
```

### Staging/Homologation
```bash
# Complete suite with moderate load
make k6-all
```

### Production (Load Testing)
```bash
# Realistic tests with high load
make k6-realistic MOBILE_VUS=20
make k6-chaos
make k6-metrics METRICS_DURATION=300s
```

## 🔍 Results Analysis

### Key Metrics to Watch

1. **Performance**:
   - `http_req_duration`: Overall latency
   - `settlement_latency_ms`: Settlement time
   - `websocket_notification_latency_ms`: Notification latency

2. **Reliability**:
   - `http_req_failed`: Overall error rate  
   - `settlement_success_rate`: Settlement success rate
   - `chaos_resilience_score`: Resilience during failures

3. **Business**:
   - `business_total_payment_volume_cents`: Financial volume processed
   - `payments_per_minute`: Business throughput
   - `ux_payment_journey_success`: User journey success

### Suggested Dashboard (Grafana)

```bash
# Dashboard metrics
- Rate: business_payments_per_minute
- Latency: http_req_duration (P50, P95, P99)
- Errors: http_req_failed rate by status
- Business: business_total_payment_volume_cents over time
- SLIs: quality_availability_sli, quality_latency_sli
```

## 🚨 Troubleshooting

### Common Issues

1. **WebSocket Timeouts**:
   - Check if WebSocket endpoint is available
   - Adjust `WS_BASE` to correct URL

2. **E2E Failures**:
   - Settlement can take time - adjust timeouts
   - Check if RabbitMQ and Ledger Service are running

3. **Chaos tests failing**:
   - This is expected! Chaos tests validate resilience
   - Watch `chaos_resilience_score` instead of error rate

4. **Docker k6 not working**:
   - Check Docker network: `--add-host=host.docker.internal:host-gateway`
   - Use `API_BASE=http://host.docker.internal:8080`

## 📈 Evolution and Customization

### Adding New Scenarios

1. **Create new file**: `k6/my_scenario.js`
2. **Follow standard structure**:
   ```javascript
   import { Counter, Trend } from "k6/metrics";
   
   const myMetric = new Counter("my_metric_total");
   
   export const options = {
     scenarios: { /* configure scenarios */ },
     thresholds: { /* define SLAs */ }
   };
   ```
3. **Add to Makefile**: Create target `k6-my-scenario`
4. **Document**: Update this README

### CI/CD Integration

```yaml
# GitHub Actions example
- name: Run k6 Smoke Tests
  run: make k6-smoke
  env:
    API_BASE: ${{ secrets.STAGING_API_URL }}
    
- name: Run k6 E2E Tests  
  run: make k6-e2e E2E_DURATION=60s
  if: github.event_name == 'pull_request'
```

---

## 📞 Support

For questions about k6 tests:
1. Check execution logs
2. Consult metrics in dashboard
3. Review this README
4. Check official k6 documentation: https://k6.io/docs/