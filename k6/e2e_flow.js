// End-to-end payment flow tests
// API Gateway -> Payment Service -> RabbitMQ -> Ledger -> Settlement -> Notification
import http from "k6/http";
import { check, group, sleep } from "k6";
import { Counter, Trend, Rate } from "k6/metrics";
import { uuidv4 } from "https://jslib.k6.io/k6-utils/1.4.0/index.js";

const base = __ENV.API_BASE || "http://127.0.0.1:8080";
const payer = __ENV.PAYER_ID || "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const payee = __ENV.PAYEE_ID || "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";

// Specific metrics for E2E flow
const e2eFlowDuration = new Trend("e2e_flow_total_duration_ms", true);
const settlementLatency = new Trend("settlement_latency_ms", true);
const settlementSuccess = new Rate("settlement_success_rate");
const timeToSettlement = new Trend("time_to_settlement_ms", true);
const pollingAttempts = new Trend("polling_attempts_until_settlement", true);

// Counters by final status
const settledPayments = new Counter("payments_settled_total");
const declinedPayments = new Counter("payments_declined_total");
const timeoutPayments = new Counter("payments_timeout_total");

export const options = {
  scenarios: {
    e2e_happy_path: {
      executor: "constant-vus",
      exec: "e2e_happy_path_test",
      vus: Number(__ENV.E2E_VUS || 3),
      duration: __ENV.E2E_DURATION || "60s",
      gracefulStop: "30s"
    },
    e2e_batch_processing: {
      executor: "shared-iterations",
      exec: "e2e_batch_test", 
      vus: 2,
      iterations: Number(__ENV.E2E_ITERATIONS || 10),
      maxDuration: "5m"
    },
    e2e_settlement_sla: {
      executor: "constant-vus",
      exec: "e2e_sla_test",
      vus: 1,
      duration: "30s",
      startTime: "10s"
    }
  },
  thresholds: {
    e2e_flow_total_duration_ms: ["p(95)<15000"],      // 95% E2E flows in < 15s
    settlement_latency_ms: ["p(90)<10000"],           // 90% settlements in < 10s
    settlement_success_rate: ["rate>0.85"],           // 85% success in settlement
    time_to_settlement_ms: ["p(50)<5000"],            // Median settlement < 5s
    polling_attempts_until_settlement: ["p(90)<15"],   // Maximum 15 polls (15s)
    payments_timeout_total: ["count<5"]               // Maximum 5 timeouts
  }
};

function createPayment(amount, description = "e2e test payment") {
  const payload = {
    idempotency_key: uuidv4(),
    amount_cents: amount,
    currency: "BRL",
    payer_id: payer,
    payee_id: payee,
    description: description
  };

  const startTime = Date.now();
  const res = http.post(`${base}/payments`, JSON.stringify(payload), {
    headers: { "Content-Type": "application/json" },
    tags: { name: "e2e_create_payment", amount_range: getAmountRange(amount) }
  });

  const createLatency = Date.now() - startTime;
  
  if (res.status === 202) {
    try {
      const payment = JSON.parse(res.body);
      return {
        ...payment,
        created_at: startTime,
        create_latency: createLatency,
        original_amount: amount
      };
    } catch (e) {
      console.error("Failed to parse payment creation response:", e);
      return null;
    }
  }
  
  console.error(`Payment creation failed: ${res.status} - ${res.body}`);
  return null;
}

function getAmountRange(amount) {
  if (amount < 10000) return "small";      // < $100
  if (amount < 100000) return "medium";    // $100 - $1.000
  return "large";                          // > $1.000
}

function waitForSettlement(paymentId, maxAttempts = 30, pollInterval = 1000) {
  const startTime = Date.now();
  let attempts = 0;
  let status = "PENDING";
  let paymentData = null;

  while (status === "PENDING" && attempts < maxAttempts) {
    attempts++;
    sleep(pollInterval / 1000); // Convert to seconds

    const res = http.get(`${base}/payments/${paymentId}`, {
      tags: { name: "e2e_poll_status", attempt: attempts }
    });

    if (res.status === 200) {
      try {
        paymentData = JSON.parse(res.body);
        status = paymentData.status;
        
        if (status !== "PENDING") {
          const settlementTime = Date.now() - startTime;
          settlementLatency.add(settlementTime);
          timeToSettlement.add(settlementTime);
          pollingAttempts.add(attempts);
          
          console.log(`Payment ${paymentId} settled to ${status} in ${settlementTime}ms after ${attempts} polls`);
          break;
        }
      } catch (e) {
        console.error(`Error parsing payment status response for ${paymentId}:`, e);
      }
    } else {
      console.error(`Failed to poll payment ${paymentId}: ${res.status}`);
    }
  }

  if (status === "PENDING") {
    timeoutPayments.add(1);
    console.error(`Payment ${paymentId} timed out after ${attempts} attempts`);
  }

  return { status, attempts, paymentData };
}

export function e2e_happy_path_test() {
  const flowStartTime = Date.now();
  
  group("E2E Payment Flow - Happy Path", () => {
    // 1. Create payment
    const amount = 5000 + Math.floor(Math.random() * 45000); // $50-$500
    const payment = createPayment(amount, `e2e happy path ${Date.now()}`);
    
    if (!payment) {
      console.error("Failed to create payment for E2E test");
      return;
    }

    check(payment, {
      "E2E: Payment created successfully": (p) => p.payment_id && p.status === "PENDING",
      "E2E: Creation latency acceptable": (p) => p.create_latency < 1000
    });

    // 2. Wait for settlement
    const settlement = waitForSettlement(payment.payment_id, 20, 1000);
    
    const flowEndTime = Date.now();
    const totalFlowTime = flowEndTime - flowStartTime;
    e2eFlowDuration.add(totalFlowTime);

    // 3. Validate settlement result
    const settlementOk = check(settlement, {
      "E2E: Settlement completed": (s) => s.status !== "PENDING",
      "E2E: Settlement within attempts limit": (s) => s.attempts <= 15,
      "E2E: Valid final status": (s) => ["SETTLED", "DECLINED"].includes(s.status)
    });

    if (settlementOk) {
      settlementSuccess.add(true);
      
      if (settlement.status === "SETTLED") {
        settledPayments.add(1);
      } else if (settlement.status === "DECLINED") {
        declinedPayments.add(1);
        
        // Log decline reason for analysis
        if (settlement.paymentData && settlement.paymentData.decline_reason) {
          console.log(`Payment ${payment.payment_id} declined: ${settlement.paymentData.decline_reason}`);
        }
      }
    } else {
      settlementSuccess.add(false);
    }

    // 4. Validate data consistency
    if (settlement.paymentData) {
      check(settlement.paymentData, {
        "E2E: Payment data consistency": (pd) => 
          pd.payment_id === payment.payment_id &&
          pd.amount_cents === payment.original_amount &&
          pd.currency === "BRL",
        "E2E: Timestamps progression": (pd) => 
          new Date(pd.created_at) <= new Date(pd.updated_at)
      });
    }

    console.log(`E2E flow completed for ${payment.payment_id}: ${settlement.status} in ${totalFlowTime}ms`);
  });

  sleep(0.5 + Math.random() * 1); // Variation in timing
}

export function e2e_batch_test() {
  group("E2E Batch Processing", () => {
    const batchSize = 5;
    const payments = [];
    const batchStartTime = Date.now();

    // 1. Create batch of payments
    for (let i = 0; i < batchSize; i++) {
      const amount = 1000 + (i * 2000); // $10, $30, $50, $70, $90
      const payment = createPayment(amount, `batch payment ${i+1}/${batchSize}`);
      
      if (payment) {
        payments.push(payment);
      }
      
      sleep(0.1); // Small delay between creations
    }

    console.log(`Created batch of ${payments.length}/${batchSize} payments`);

    // 2. Wait for settlement of all payments
    const settlements = [];
    for (const payment of payments) {
      const settlement = waitForSettlement(payment.payment_id, 25, 800);
      settlements.push({
        payment_id: payment.payment_id,
        ...settlement
      });
    }

    const batchEndTime = Date.now();
    const batchProcessingTime = batchEndTime - batchStartTime;

    // 3. Batch analysis
    const settledCount = settlements.filter(s => s.status === "SETTLED").length;
    const declinedCount = settlements.filter(s => s.status === "DECLINED").length;
    const timeoutCount = settlements.filter(s => s.status === "PENDING").length;

    check(settlements, {
      "Batch: Majority settled successfully": () => settledCount >= (batchSize * 0.6),
      "Batch: No timeouts": () => timeoutCount === 0,
      "Batch: Processing time reasonable": () => batchProcessingTime < 30000
    });

    console.log(`Batch results: ${settledCount} settled, ${declinedCount} declined, ${timeoutCount} timeout`);
    console.log(`Batch processing time: ${batchProcessingTime}ms`);
  });
}

export function e2e_sla_test() {
  group("E2E SLA Compliance", () => {
    // Test focused on validating specific SLAs
    const slaTestAmount = 25000; // $250 - medium value
    const payment = createPayment(slaTestAmount, "SLA compliance test");
    
    if (!payment) {
      console.error("Failed to create payment for SLA test");
      return;
    }

    const slaStartTime = Date.now();
    
    // SLA: Settlement in up to 10 seconds
    const settlement = waitForSettlement(payment.payment_id, 15, 600);
    
    const slaEndTime = Date.now();
    const slaTime = slaEndTime - slaStartTime;

    const slaCompliant = check(settlement, {
      "SLA: Settlement within 10s": () => slaTime <= 10000,
      "SLA: Settlement successful": (s) => s.status !== "PENDING",
      "SLA: Max 10 polling attempts": (s) => s.attempts <= 10
    });

    if (slaCompliant) {
      console.log(`✅ SLA compliant: ${payment.payment_id} settled in ${slaTime}ms`);
    } else {
      console.log(`❌ SLA violation: ${payment.payment_id} took ${slaTime}ms`);
    }
  });

  sleep(2); // Pause between SLA tests
}