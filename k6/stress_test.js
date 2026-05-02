// Stress Test - Gradual load until system limit is found
import http from "k6/http";
import ws from "k6/ws";
import { check, group, sleep } from "k6";
import { Counter, Trend, Rate, Gauge } from "k6/metrics";
import { uuidv4 } from "https://jslib.k6.io/k6-utils/1.4.0/index.js";

const base = __ENV.API_BASE || "http://127.0.0.1:8080";
const wsBase = __ENV.WS_BASE || "ws://127.0.0.1:8080";
const payer = __ENV.PAYER_ID || "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const payee = __ENV.PAYEE_ID || "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";

// Specific stress metrics
const stressPayments = new Counter("stress_payments_total");
const stressErrors = new Counter("stress_errors_total");
const stressLatency = new Trend("stress_latency_ms", true);
const stressSuccessRate = new Rate("stress_success_rate");
const concurrentUsers = new Gauge("concurrent_users");

// Metrics by stress phase
const warmupMetrics = new Counter("warmup_requests");
const rampUpMetrics = new Counter("rampup_requests"); 
const peakMetrics = new Counter("peak_requests");
const breakingPointMetrics = new Counter("breaking_point_requests");

// Resource metrics (simulated)
const cpuUsage = new Gauge("estimated_cpu_usage_percent");
const memoryUsage = new Gauge("estimated_memory_usage_mb");

export const options = {
  scenarios: {
    // Phase 1: Warmup - Verify system is working
    warmup_phase: {
      executor: "constant-vus",
      exec: "warmup_test",
      vus: 5,
      duration: "2m",
      gracefulStop: "30s",
      tags: { phase: "warmup" }
    },

    // Phase 2: Gradual ramp-up - Find normal capacity
    gradual_ramp: {
      executor: "ramping-vus", 
      exec: "stress_payment_flow",
      startVUs: 0,
      stages: [
        { duration: "2m", target: 20 },   // Initial growth
        { duration: "3m", target: 50 },   // Moderate load
        { duration: "3m", target: 100 },  // High load
        { duration: "2m", target: 150 },  // Very high load
        { duration: "2m", target: 200 },  // Expected limit
      ],
      startTime: "2m", // After warmup
      gracefulRampDown: "1m",
      tags: { phase: "ramp" }
    },

    // Phase 3: Peak stress - Test breaking point
    peak_stress: {
      executor: "ramping-vus",
      exec: "intensive_payment_burst", 
      startVUs: 0,
      stages: [
        { duration: "1m", target: 300 },   // Rapid spike
        { duration: "3m", target: 500 },   // Sustained peak
        { duration: "2m", target: 750 },   // Stress extremo
        { duration: "1m", target: 1000 },  // Breaking point
        { duration: "2m", target: 500 },   // Recovery
        { duration: "1m", target: 0 }      // Cooldown
      ],
      startTime: "12m", // After ramp-up
      gracefulRampDown: "2m",
      tags: { phase: "peak" }
    },

    // Phase 4: WebSocket stress - Concurrent connections
    websocket_stress: {
      executor: "constant-vus",
      exec: "websocket_connection_stress",
      vus: Number(__ENV.WS_STRESS_VUS || 100),
      duration: "10m",
      startTime: "5m", // Parallel to ramp-up
      gracefulStop: "1m",
      tags: { phase: "websocket" }
    }
  },

  thresholds: {
    // Thresholds to detect when system breaks
    "http_req_duration": [
      "p(50)<500",     // 50% of requests < 500ms
      "p(90)<2000",    // 90% of requests < 2s  
      "p(99)<5000"     // 99% of requests < 5s
    ],
    "http_req_failed": ["rate<0.1"],  // < 10% de erro
    "stress_success_rate": ["rate>0.85"], // > 85% success
    "stress_latency_ms": ["p(95)<3000"],  // 95% < 3s
    "checks": ["rate>0.8"] // 80% of checks passing
  }
};

export function warmup_test() {
  concurrentUsers.add(1);
  
  group("Warmup - Health Check", () => {
    const healthRes = http.get(`${base}/health`);
    check(healthRes, {
      "health endpoint ok": (r) => r.status === 200,
    });
    warmupMetrics.add(1);
  });

  group("Warmup - Simple Payment", () => {
    const paymentRes = createPayment(100); // $1.00
    const success = check(paymentRes, {
      "warmup payment created": (r) => r.status === 201,
      "warmup payment has id": (r) => r.json("payment_id") !== undefined,
    });
    
    warmupMetrics.add(1);
    stressSuccessRate.add(success);
  });
  
  sleep(1); // Interval between requests in warmup
}

export function stress_payment_flow() {
  concurrentUsers.add(__VU);
  
  const startTime = Date.now();
  
  group("Stress Payment Flow", () => {
    // Scenario 1: Simple payment (70% of traffic)
    if (Math.random() < 0.7) {
      const amount = Math.floor(Math.random() * 50000) + 100; // $1 - $500
      const paymentRes = createPayment(amount);
      
      const success = check(paymentRes, {
        "payment created or rate-limited": (r) => r.status === 201 || r.status === 429,
        "no server error": (r) => r.status < 500,
        "payment id exists": (r) => r.status !== 201 || r.json("payment_id") !== undefined,
      });
      
      stressPayments.add(1);
      rampUpMetrics.add(1);
      stressSuccessRate.add(success);
      
      if (!success) {
        stressErrors.add(1);
      }
    }
    
    // Scenario 2: Payment + Status check (20% of traffic)
    else if (Math.random() < 0.9) {
      const amount = Math.floor(Math.random() * 100000) + 1000; // $10 - $1000
      const paymentRes = createPayment(amount);
      
      if (paymentRes.status === 201) {
        const paymentId = paymentRes.json("payment_id");
        
        // Verify status after creation
        sleep(0.1); // Wait for processing
        const statusRes = http.get(`${base}/payments/${paymentId}`);
        
        const success = check(statusRes, {
          "status check ok": (r) => r.status === 200,
          "status is valid": (r) => ["PENDING", "SETTLED", "DECLINED"].includes(r.json("status")),
        });
        
        stressSuccessRate.add(success);
      }
      
      stressPayments.add(1);
      rampUpMetrics.add(1);
    }
    
    // Scenario 3: Batch of payments (10% of traffic)
    else {
      for (let i = 0; i < 3; i++) {
        const amount = Math.floor(Math.random() * 10000) + 500; // $5 - $100
        createPayment(amount);
        stressPayments.add(1);
      }
      rampUpMetrics.add(3);
    }
  });
  
  const duration = Date.now() - startTime;
  stressLatency.add(duration);
  
  // Simulate system metrics based on load
  const load = __VU / 100; // Approximation of load
  cpuUsage.add(Math.min(20 + (load * 60), 100));
  memoryUsage.add(Math.min(256 + (load * 512), 2048));
  
  // Interval based on current load
  const sleepTime = Math.max(0.1, 1 - (load * 0.8));
  sleep(sleepTime);
}

export function intensive_payment_burst() {
  concurrentUsers.add(__VU);
  
  group("Peak Stress - Payment Burst", () => {
    // Burst of 3-5 payments per VU
    const burstSize = Math.floor(Math.random() * 3) + 3;
    
    for (let i = 0; i < burstSize; i++) {
      const amount = Math.floor(Math.random() * 200000) + 1000; // $10 - $2000
      const paymentRes = createPayment(amount);
      
      // 429 = rate limited (expected under high load, not a system failure).
    // 201 = created. Anything else = real error.
    const success = check(paymentRes, {
        "burst payment ok": (r) => r.status === 201 || r.status === 429,
        "no server error":  (r) => r.status < 500,
      });
      
      stressPayments.add(1);
      peakMetrics.add(1);
      breakingPointMetrics.add(1);
      stressSuccessRate.add(success);
      
      if (!success) {
        stressErrors.add(1);
      }
      
      // Minimum interval between requests in burst
      sleep(0.05);
    }
  });
  
  // Simulate high load in the system
  cpuUsage.add(Math.min(50 + (__VU / 10), 100));
  memoryUsage.add(Math.min(512 + (__VU * 2), 4096));
  
  // No sleep - maximum pressure
}

export function websocket_connection_stress() {
  const paymentId = uuidv4();
  
  ws.connect(`${wsBase}/ws?payment_id=${paymentId}`, null, (socket) => {
    socket.on("open", () => {
      console.log(`WebSocket opened for payment ${paymentId}`);
    });
    
    socket.on("message", (data) => {
      const message = JSON.parse(data);
      check(message, {
        "websocket message valid": (m) => m.payment_id === paymentId,
        "websocket status valid": (m) => ["PENDING", "SETTLED", "DECLINED"].includes(m.status),
      });
    });
    
    socket.on("error", (e) => {
      console.log(`WebSocket error: ${e}`);
      stressErrors.add(1);
    });
    
    // Keep connection alive
    socket.setInterval(() => {
      socket.ping();
    }, 30000);
  });
  
  // WebSocket stress - keep connection for entire duration
  sleep(600); // 10 minutes
}

// Helper function to create payments
function createPayment(amountCents) {
  const payload = {
    payer_id: payer,
    payee_id: payee,
    amount_cents: amountCents,
    currency: "BRL",
    idempotency_key: uuidv4(),
  };

  const params = {
    headers: { 
      "Content-Type": "application/json",
      "User-Agent": `k6-stress-test/${__VU}`,
    },
  };

  return http.post(`${base}/payments`, JSON.stringify(payload), params);
}

export function handleSummary(data) {
  return {
    'stress-test-summary.json': JSON.stringify(data, null, 2),
    stdout: `
╔══════════════════════════════════════════════════════════╗
║                    STRESS TEST RESULTS                   ║
╠══════════════════════════════════════════════════════════╣
║ Total Payments: ${data.metrics.stress_payments_total?.values.count || 0}
║ Success Rate: ${((data.metrics.stress_success_rate?.values.rate || 0) * 100).toFixed(2)}%
║ Average Latency: ${(data.metrics.stress_latency_ms?.values.avg || 0).toFixed(2)}ms
║ P95 Latency: ${(data.metrics.stress_latency_ms?.values['p(95)'] || 0).toFixed(2)}ms
║ Peak Concurrent Users: ${data.metrics.concurrent_users?.values.max || 0}
║ Total Errors: ${data.metrics.stress_errors_total?.values.count || 0}
║
║ Phases Completed:
║ • Warmup: ${data.metrics.warmup_requests?.values.count || 0} requests
║ • Ramp-up: ${data.metrics.rampup_requests?.values.count || 0} requests  
║ • Peak: ${data.metrics.peak_requests?.values.count || 0} requests
║ • Breaking Point: ${data.metrics.breaking_point_requests?.values.count || 0} requests
║
║ System Breaking Point Detection:
║ • Failed Requests: ${((data.metrics.http_req_failed?.values.rate || 0) * 100).toFixed(2)}%
║ • P99 Latency: ${(data.metrics.http_req_duration?.values['p(99)'] || 0).toFixed(2)}ms
║ • Max Est. CPU: ${data.metrics.estimated_cpu_usage_percent?.values.max || 0}%
║ • Max Est. Memory: ${data.metrics.estimated_memory_usage_mb?.values.max || 0}MB
╚══════════════════════════════════════════════════════════╝
    `
  };
}