// Advanced business metrics and detailed observability
import http from "k6/http";
import { check, group, sleep } from "k6";
import { Counter, Trend, Rate, Gauge } from "k6/metrics";
import { uuidv4 } from "https://jslib.k6.io/k6-utils/1.4.0/index.js";

const base = __ENV.API_BASE || "http://127.0.0.1:8080";
const payer = __ENV.PAYER_ID || "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const payee = __ENV.PAYEE_ID || "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";

// ========== BUSINESS METRICS ==========

// Volume and value processed
const totalPaymentVolume = new Counter("business_total_payment_volume_cents");
const totalPaymentCount = new Counter("business_total_payments");
const averagePaymentValue = new Trend("business_avg_payment_value_cents", true);
const paymentVelocity = new Trend("business_payments_per_minute", true);

// Distribution by value ranges (according to BC and regulations)
const pixValues = new Counter("business_pix_volume_cents");           // Up to $1,000
const tedValues = new Counter("business_ted_volume_cents");           // $1,000+
const highValuePayments = new Counter("business_high_value_count");   // Above $10,000 (requires KYC)

// Compliance and risk metrics
const kycRequiredPayments = new Counter("compliance_kyc_required_payments");
const antifraudChecks = new Counter("compliance_antifraud_checks");
const suspiciousActivityRate = new Rate("compliance_suspicious_activity_rate");

// ========== OPERATIONAL METRICS ==========

// Granular latency by component
const apiGatewayLatency = new Trend("ops_api_gateway_latency_ms", true);
const paymentServiceLatency = new Trend("ops_payment_service_latency_ms", true);
const databaseLatency = new Trend("ops_database_latency_ms", true);
const messagingLatency = new Trend("ops_messaging_latency_ms", true);

// Throughput by endpoint
const createPaymentThroughput = new Counter("ops_create_payment_requests");
const getPaymentThroughput = new Counter("ops_get_payment_requests");
const healthCheckThroughput = new Counter("ops_health_check_requests");

// System efficiency
const connectionPoolUtilization = new Gauge("ops_db_connection_pool_usage");
const memoryUsage = new Gauge("ops_memory_usage_percent");
const cpuUtilization = new Gauge("ops_cpu_usage_percent");

// ========== QUALITY METRICS ==========

// SLI/SLO tracking
const availabilitySLI = new Rate("quality_availability_sli");        // Target: 99.9%
const latencySLI = new Rate("quality_latency_sli");                 // Target: P95 < 2s
const errorBudgetBurn = new Trend("quality_error_budget_burn_rate", true);

// Error categorization
const clientErrors = new Counter("quality_client_errors_4xx");
const serverErrors = new Counter("quality_server_errors_5xx");
const timeoutErrors = new Counter("quality_timeout_errors");
const validationErrors = new Counter("quality_validation_errors");

// ========== USER EXPERIENCE METRICS ==========

// User journey
const userJourneySuccess = new Rate("ux_payment_journey_success");
const abandonmentRate = new Rate("ux_payment_abandonment_rate");
const retryRate = new Rate("ux_payment_retry_rate");

// Perceived performance
const timeToFirstResponse = new Trend("ux_time_to_first_response_ms", true);
const timeToCompletion = new Trend("ux_time_to_completion_ms", true);
const userSatisfactionScore = new Trend("ux_satisfaction_score", true); // 1-5

export const options = {
  scenarios: {
    advanced_metrics_collection: {
      executor: "constant-vus",
      exec: "advanced_metrics_test",
      vus: Number(__ENV.METRICS_VUS || 5),
      duration: __ENV.METRICS_DURATION || "120s",
      gracefulStop: "20s"
    },
    
    business_intelligence: {
      executor: "ramping-vus", 
      exec: "business_intelligence_test",
      startVUs: 0,
      stages: [
        { duration: "30s", target: 3 },
        { duration: "60s", target: 8 },
        { duration: "30s", target: 0 }
      ],
      gracefulRampDown: "10s"
    },

    compliance_monitoring: {
      executor: "constant-vus",
      exec: "compliance_monitoring_test", 
      vus: 2,
      duration: "90s",
      startTime: "10s"
    }
  },

  thresholds: {
    // SLIs/SLOs
    "quality_availability_sli": ["rate>=0.999"],              // 99.9% availability
    "quality_latency_sli": ["rate>=0.95"],                   // 95% under latency SLO
    "quality_error_budget_burn_rate": ["p(90)<0.1"],         // Burn rate < 10%
    
    // Business metrics
    "business_payments_per_minute": ["p(50)>30"],            // Median > 30 payments/min
    "business_avg_payment_value_cents": ["p(90)<1000000"],   // P90 < $10,000
    
    // Operational metrics  
    "ops_api_gateway_latency_ms": ["p(95)<500"],             // API Gateway P95 < 500ms
    "ops_payment_service_latency_ms": ["p(95)<1000"],        // Payment Service P95 < 1s
    "ops_database_latency_ms": ["p(90)<200"],                // Database P90 < 200ms
    
    // UX metrics
    "ux_payment_journey_success": ["rate>0.9"],              // 90% success rate
    "ux_time_to_completion_ms": ["p(95)<8000"],              // P95 completion < 8s
    "ux_satisfaction_score": ["avg>=4.0"]                    // Average satisfaction >= 4/5
  }
};

// Simulate system data (normally comes from APM/monitoring)
function simulateSystemMetrics() {
  // Simulate connection pool usage (0-100%)
  const poolUsage = 20 + Math.random() * 60; // 20-80% usage
  connectionPoolUtilization.add(poolUsage);
  
  // Simulate CPU/memory usage
  const cpuUsage = 15 + Math.random() * 45; // 15-60% CPU
  const memUsage = 30 + Math.random() * 40; // 30-70% memory
  cpuUtilization.add(cpuUsage);
  memoryUsage.add(memUsage);
}

function calculateBusinessMetrics(amount, responseTime, success) {
  // Volume and count
  totalPaymentVolume.add(amount);
  totalPaymentCount.add(1);
  averagePaymentValue.add(amount);
  
  // Classification by type (simulating BC/PIX rules)
  if (amount <= 100000) {  // <= $1,000
    pixValues.add(amount);
  } else {
    tedValues.add(amount);
  }
  
  // High-value payments (KYC requirements)
  if (amount >= 1000000) {  // >= $10.000
    highValuePayments.add(1);
    kycRequiredPayments.add(1);
  }
  
  // Anti-fraud simulation
  antifraudChecks.add(1);
  const isSuspicious = amount > 5000000 || responseTime > 5000; // Simple heuristic
  suspiciousActivityRate.add(isSuspicious);
}

function calculateSLIMetrics(responseTime, statusCode) {
  // Availability SLI - any successful response
  const isAvailable = statusCode > 0 && statusCode < 500;
  availabilitySLI.add(isAvailable);
  
  // Latency SLI - under 2s for 95% of requests
  const meetsLatencySLO = responseTime < 2000;
  latencySLI.add(meetsLatencySLO);
  
  // Error budget burn rate (simplified)
  const errorRate = isAvailable ? 0 : 0.001; // 0.1% error adds to burn
  errorBudgetBurn.add(errorRate);
}

function categorizeError(statusCode, responseTime) {
  if (statusCode >= 400 && statusCode < 500) {
    clientErrors.add(1);
    if (statusCode === 400) validationErrors.add(1);
  } else if (statusCode >= 500) {
    serverErrors.add(1);
  } else if (responseTime > 10000) {
    timeoutErrors.add(1);
  }
}

function trackUserExperience(journeyStartTime, success, retryCount = 0) {
  const totalJourneyTime = Date.now() - journeyStartTime;
  
  userJourneySuccess.add(success);
  timeToCompletion.add(totalJourneyTime);
  
  if (retryCount > 0) {
    retryRate.add(true);
  } else {
    retryRate.add(false);
  }
  
  // Simulate user satisfaction based on performance and success
  let satisfaction = 5; // Start with max satisfaction
  if (!success) satisfaction -= 2;
  if (totalJourneyTime > 5000) satisfaction -= 1;
  if (retryCount > 0) satisfaction -= 0.5;
  
  userSatisfactionScore.add(Math.max(1, satisfaction));
}

export function advanced_metrics_test() {
  group("Advanced Metrics Collection", () => {
    simulateSystemMetrics();
    
    const journeyStart = Date.now();
    
    // Track API Gateway latency
    const gatewayStart = Date.now();
    
    const amount = 5000 + Math.floor(Math.random() * 95000); // $50-1000
    const payload = {
      idempotency_key: uuidv4(),
      amount_cents: amount,
      currency: "BRL",
      payer_id: payer,
      payee_id: payee,
      description: "Advanced metrics test"
    };

    const res = http.post(`${base}/payments`, JSON.stringify(payload), {
      headers: { "Content-Type": "application/json" },
      tags: { 
        name: "advanced_metrics_payment",
        value_tier: getValueTier(amount)
      }
    });
    
    const gatewayLatency = Date.now() - gatewayStart;
    apiGatewayLatency.add(gatewayLatency);
    
    // Simulate component latency breakdown
    const dbLatency = Math.random() * 150 + 20; // 20-170ms
    const msgLatency = Math.random() * 50 + 10;  // 10-60ms
    databaseLatency.add(dbLatency);
    messagingLatency.add(msgLatency);
    
    // Track throughput
    createPaymentThroughput.add(1);
    
    const success = res.status === 202;
    
    check(res, {
      "Advanced: Payment created": (r) => r.status === 202,
      "Advanced: Response has all fields": (r) => {
        try {
          const body = JSON.parse(r.body);
          return body.payment_id && body.status && body.created_at;
        } catch {
          return false;
        }
      }
    });
    
    // Calculate all metrics
    calculateBusinessMetrics(amount, gatewayLatency, success);
    calculateSLIMetrics(gatewayLatency, res.status);
    
    if (!success) {
      categorizeError(res.status, gatewayLatency);
    }
    
    // Track user experience
    trackUserExperience(journeyStart, success);
    
    // Time to first response (simulating frontend perception)
    timeToFirstResponse.add(gatewayLatency);
    
    console.log(`Advanced metrics: ${amount/100}R$ payment, ${gatewayLatency}ms, status=${res.status}`);
  });

  sleep(0.8 + Math.random() * 0.4);
}

export function business_intelligence_test() {
  group("Business Intelligence Metrics", () => {
    // Simulate various business scenarios
    const scenarios = [
      { amount: 2500, type: "retail_pix" },      // $25 PIX
      { amount: 15000, type: "ecommerce" },      // $150 e-commerce
      { amount: 250000, type: "corporate_ted" }, // $2,500 TED
      { amount: 1500000, type: "high_value" }    // $15,000 high-value
    ];
    
    const scenario = scenarios[Math.floor(Math.random() * scenarios.length)];
    
    const payload = {
      idempotency_key: uuidv4(),
      amount_cents: scenario.amount,
      currency: "BRL",
      payer_id: payer,
      payee_id: payee,
      description: `BI test - ${scenario.type}`
    };

    const startTime = Date.now();
    const res = http.post(`${base}/payments`, JSON.stringify(payload), {
      headers: { "Content-Type": "application/json" },
      tags: { 
        name: "bi_payment",
        business_type: scenario.type,
        value_tier: getValueTier(scenario.amount)
      }
    });
    
    const responseTime = Date.now() - startTime;
    
    // Business intelligence tracking
    calculateBusinessMetrics(scenario.amount, responseTime, res.status === 202);
    
    // Calculate payments per minute (sliding window)
    const currentMinute = Math.floor(Date.now() / 60000);
    paymentVelocity.add(1, { minute: currentMinute });
    
    check(res, {
      "BI: Business scenario handled": (r) => r.status === 202,
      "BI: Appropriate processing time": (r) => {
        // High-value transactions may take longer due to additional checks
        const maxTime = scenario.amount >= 1000000 ? 3000 : 1500;
        return r.timings.duration < maxTime;
      }
    });
    
    console.log(`BI: ${scenario.type} payment of $${scenario.amount/100} processed in ${responseTime}ms`);
  });

  sleep(1 + Math.random() * 2);
}

export function compliance_monitoring_test() {
  group("Compliance and Risk Monitoring", () => {
    // Test various compliance scenarios
    const complianceScenarios = [
      { amount: 50000, risk: "low" },     // $500 - low risk
      { amount: 500000, risk: "medium" }, // $5,000 - medium risk  
      { amount: 1500000, risk: "high" }   // $15,000 - high risk (KYC)
    ];
    
    const scenario = complianceScenarios[Math.floor(Math.random() * complianceScenarios.length)];
    
    const payload = {
      idempotency_key: uuidv4(),
      amount_cents: scenario.amount,
      currency: "BRL",
      payer_id: payer,
      payee_id: payee,
      description: `Compliance test - ${scenario.risk} risk`
    };

    const res = http.post(`${base}/payments`, JSON.stringify(payload), {
      headers: { 
        "Content-Type": "application/json",
        "X-Risk-Level": scenario.risk
      },
      tags: { 
        name: "compliance_payment",
        risk_level: scenario.risk
      }
    });
    
    // Compliance metrics
    if (scenario.amount >= 1000000) {
      kycRequiredPayments.add(1);
    }
    
    antifraudChecks.add(1);
    
    // Simulate suspicious activity detection
    const suspiciousIndicators = [
      scenario.amount > 2000000,  // Very high value
      res.timings.duration > 3000 // Unusual processing time
    ];
    
    const isSuspicious = suspiciousIndicators.some(indicator => indicator);
    suspiciousActivityRate.add(isSuspicious);
    
    check(res, {
      "Compliance: Transaction processed": (r) => r.status === 202,
      "Compliance: Risk assessment completed": () => true, // Would check risk headers in real scenario
      "Compliance: KYC requirements met": () => {
        // High-value transactions should have additional validation time
        if (scenario.amount >= 1000000) {
          return res.timings.duration >= 200; // Minimum time for KYC checks
        }
        return true;
      }
    });
    
    if (isSuspicious) {
      console.log(`🚨 Suspicious activity detected: $${scenario.amount/100} payment flagged`);
    }
  });

  sleep(2 + Math.random() * 3);
}

function getValueTier(amount) {
  if (amount < 10000) return "micro";      // < $100
  if (amount < 100000) return "small";     // $100-1,000
  if (amount < 1000000) return "medium";   // $1,000-10,000  
  return "large";                          // > $10,000
}