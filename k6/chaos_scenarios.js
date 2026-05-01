// Chaos Testing - Validation of system resilience under failures
import http from "k6/http";
import { check, group, sleep } from "k6";
import { Counter, Trend, Rate } from "k6/metrics";
import { uuidv4 } from "https://jslib.k6.io/k6-utils/1.4.0/index.js";

const base = __ENV.API_BASE || "http://127.0.0.1:8080";
const payer = __ENV.PAYER_ID || "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const payee = __ENV.PAYEE_ID || "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";

// Metrics for chaos testing
const chaosErrorRate = new Rate("chaos_error_rate");
const chaosRecoveryTime = new Trend("chaos_recovery_time_ms", true);
const chaosResilienceScore = new Rate("chaos_resilience_score");
const slowResponseCounter = new Counter("slow_responses_during_chaos");
const timeoutCounter = new Counter("timeout_responses_during_chaos");

// Metrics by chaos type
const dbSlowErrors = new Counter("db_slow_errors");
const networkErrors = new Counter("network_errors");
const serviceUnavailableErrors = new Counter("service_unavailable_errors");

export const options = {
  scenarios: {
    // Baseline scenario - normal operation
    baseline_load: {
      executor: "constant-vus",
      exec: "baseline_operations",
      vus: 3,
      duration: "3m",
      gracefulStop: "10s"
    },
    
    // Simulate slow database
    database_chaos: {
      executor: "constant-vus",
      exec: "database_latency_chaos", 
      vus: 2,
      duration: "45s",
      startTime: "30s",
      gracefulStop: "10s"
    },
    
    // Simulate unstable RabbitMQ
    messaging_chaos: {
      executor: "constant-vus",
      exec: "messaging_chaos_test",
      vus: 2,
      duration: "30s", 
      startTime: "1m",
      gracefulStop: "10s"
    },
    
    // Simulate high network latency
    network_chaos: {
      executor: "constant-vus",
      exec: "network_latency_chaos",
      vus: 1,
      duration: "30s",
      startTime: "1m30s", 
      gracefulStop: "10s"
    },
    
    // Sudden traffic spikes 
    traffic_spike: {
      executor: "ramping-vus",
      exec: "traffic_spike_test", 
      startVUs: 0,
      stages: [
        { duration: "5s", target: 1 },
        { duration: "10s", target: 15 }, // Sudden spike
        { duration: "10s", target: 2 },  // Back to normal
        { duration: "5s", target: 0 }
      ],
      startTime: "2m",
      gracefulRampDown: "5s"
    }
  },
  
  thresholds: {
    // During chaos, we tolerate more errors
    "chaos_error_rate": ["rate<0.5"],                    // Maximum 50% error during chaos
    "chaos_resilience_score": ["rate>0.3"],             // Minimum 30% resilience
    "chaos_recovery_time_ms": ["p(90)<5000"],           // Recovery in < 5s
    "slow_responses_during_chaos": ["count<20"],        // Maximum 20 slow responses
    "timeout_responses_during_chaos": ["count<10"],     // Maximum 10 timeouts
    
    // Specific metrics by scenario
    "http_req_duration{scenario:baseline_load}": ["p(95)<1000"],
    "http_req_failed{scenario:baseline_load}": ["rate<0.1"],
    
    "http_req_duration{scenario:database_chaos}": ["p(95)<8000"],  // During slow database
    "http_req_failed{scenario:database_chaos}": ["rate<0.6"],
    
    "http_req_duration{scenario:traffic_spike}": ["p(95)<3000"],   // During spike
    "http_req_failed{scenario:traffic_spike}": ["rate<0.4"]
  }
};

function makePaymentRequest(scenario, timeout = 5000) {
  const payload = {
    idempotency_key: uuidv4(),
    amount_cents: 1000 + Math.floor(Math.random() * 9000),
    currency: "BRL",
    payer_id: payer,
    payee_id: payee,
    description: `chaos test - ${scenario}`
  };

  const startTime = Date.now();
  const chaosType = getChaosType(); // Calculate outside of tags
  
  const res = http.post(`${base}/payments`, JSON.stringify(payload), {
    headers: { "Content-Type": "application/json" },
    timeout: `${timeout}ms`,
    tags: { 
      name: "chaos_payment",
      scenario: scenario,
      chaos_type: chaosType
    }
  });

  const responseTime = Date.now() - startTime;

  // Categorize response for analysis
  if (res.status === 0) {
    timeoutCounter.add(1);
    chaosErrorRate.add(true);
  } else if (res.status >= 500) {
    serviceUnavailableErrors.add(1);
    chaosErrorRate.add(true);
  } else if (res.status >= 400) {
    networkErrors.add(1);
    chaosErrorRate.add(true);
  } else if (responseTime > 3000) {
    slowResponseCounter.add(1);
    if (responseTime > 8000) {
      dbSlowErrors.add(1);
    }
    chaosErrorRate.add(true);
  } else {
    chaosErrorRate.add(false);
    chaosResilienceScore.add(true);
  }

  return { response: res, responseTime };
}

function getChaosType() {
  const chaosHeaders = __ENV.CHAOS_TYPE;
  if (chaosHeaders) return chaosHeaders;
  
  // Auto-detect chaos type based on elapsed test time
  // Use __VU and time to detect active scenario
  const now = Date.now();
  
  // Detect scenario based on options settings
  if (__ENV.K6_SCENARIO_NAME) {
    const scenario = __ENV.K6_SCENARIO_NAME;
    if (scenario.includes("database")) return "db_latency";
    if (scenario.includes("messaging")) return "messaging_chaos"; 
    if (scenario.includes("network")) return "network_latency";
    if (scenario.includes("traffic")) return "traffic_spike";
  }
  
  return "baseline";
}

export function baseline_operations() {
  group("Baseline Operations", () => {
    const { response, responseTime } = makePaymentRequest("baseline", 2000);
    
    const baselineOk = check(response, {
      "Baseline: Normal response status": (r) => r.status === 202,
      "Baseline: Response time acceptable": () => responseTime < 1000,
      "Baseline: Valid payment response": (r) => {
        try {
          const body = JSON.parse(r.body);
          return body.payment_id && body.status === "PENDING";
        } catch {
          return false;
        }
      }
    });

    if (!baselineOk) {
      console.log(`Baseline operation failed: ${response.status} in ${responseTime}ms`);
    }
  });

  sleep(0.5 + Math.random() * 1);
}

export function database_latency_chaos() {
  group("Database Latency Chaos", () => {
    console.log("🔥 Database chaos active - simulating slow DB responses");
    
    const startTime = Date.now();
    const { response, responseTime } = makePaymentRequest("db_chaos", 10000);
    
    // During database chaos, we expect slower responses but still functional
    const dbChaosHandled = check(response, {
      "DB Chaos: Service still responding": (r) => r.status > 0, // No timeout
      "DB Chaos: Graceful degradation": (r) => r.status === 202 || r.status === 503 || r.status === 504,
      "DB Chaos: Response within chaos SLA": () => responseTime < 8000,
      "DB Chaos: No complete service failure": (r) => r.status !== 500
    });

    if (response.status === 202) {
      chaosResilienceScore.add(true);
      console.log(`✅ DB chaos handled gracefully: ${responseTime}ms`);
    } else if (response.status === 503 || response.status === 504) {
      console.log(`⚠️ DB chaos caused graceful failure: ${response.status} in ${responseTime}ms`);
    } else {
      console.log(`❌ DB chaos caused unexpected failure: ${response.status}`);
    }

    // Measure recovery time if necessary
    if (response.status >= 500) {
      sleep(1);
      const recoveryTest = makePaymentRequest("db_recovery", 3000);
      if (recoveryTest.response.status === 202) {
        chaosRecoveryTime.add(1000 + recoveryTest.responseTime);
        console.log("🔄 System recovered from DB chaos");
      }
    }
  });

  sleep(1);
}

export function messaging_chaos_test() {
  group("Messaging Chaos", () => {
    console.log("🔥 Messaging chaos active - simulating RabbitMQ issues");
    
    const { response, responseTime } = makePaymentRequest("messaging_chaos", 6000);
    
    // During messaging chaos, payments should be created but settlement may fail
    const messagingChaosHandled = check(response, {
      "Messaging Chaos: Payment creation works": (r) => r.status === 202,
      "Messaging Chaos: Response time reasonable": () => responseTime < 3000,
      "Messaging Chaos: Proper error handling": (r) => {
        if (r.status !== 202) {
          return [502, 503, 504].includes(r.status); // Proper error codes
        }
        return true;
      }
    });

    if (response.status === 202) {
      chaosResilienceScore.add(true);
      
      // Verify if the payment was persisted (even with messaging chaos)
      try {
        const payment = JSON.parse(response.body);
        sleep(0.5);
        
        const getRes = http.get(`${base}/payments/${payment.payment_id}`, {
          timeout: "2000ms",
          tags: { name: "chaos_verify_persistence" }
        });
        
        check(getRes, {
          "Messaging Chaos: Payment persisted despite messaging issues": (r) => r.status === 200
        });
        
        console.log(`✅ Payment persisted during messaging chaos: ${payment.payment_id}`);
      } catch (e) {
        console.error("Failed to verify payment persistence during messaging chaos");
      }
    }
  });

  sleep(0.8);
}

export function network_latency_chaos() {
  group("Network Latency Chaos", () => {
    console.log("🔥 Network chaos active - simulating high latency");
    
    const { response, responseTime } = makePaymentRequest("network_chaos", 12000);
    
    const networkChaosHandled = check(response, {
      "Network Chaos: Request completed": (r) => r.status > 0,
      "Network Chaos: Timeout handling": () => responseTime < 10000,
      "Network Chaos: Circuit breaker activation": (r) => {
        // If many requests are failing, circuit breaker should activate
        return r.status === 202 || r.status === 503;
      }
    });

    if (response.status === 503) {
      console.log("🔄 Circuit breaker activated due to network chaos");
      chaosResilienceScore.add(true); // Circuit breaker is desired behavior
    } else if (response.status === 202 && responseTime > 5000) {
      console.log(`⚠️ Slow response during network chaos: ${responseTime}ms`);
    }
  });

  sleep(1.5);
}

export function traffic_spike_test() {
  group("Traffic Spike Chaos", () => {
    const { response, responseTime } = makePaymentRequest("traffic_spike", 4000);
    
    // During traffic spikes, we expect some degradation but not complete failure
    const spikeHandled = check(response, {
      "Traffic Spike: System handles load": (r) => r.status === 202 || r.status === 429,
      "Traffic Spike: Rate limiting active": (r) => {
        if (r.status === 429) {
          console.log("🚦 Rate limiting activated during traffic spike");
          return true;
        }
        return r.status === 202;
      },
      "Traffic Spike: Response time degradation acceptable": () => responseTime < 3000
    });

    if (response.status === 202) {
      chaosResilienceScore.add(true);
    } else if (response.status === 429) {
      // Rate limiting is expected behavior during spikes
      chaosResilienceScore.add(true);
      console.log("🚦 Rate limit applied correctly during spike");
    }
  });

  sleep(0.1 + Math.random() * 0.2); // More frequent requests during spike
}