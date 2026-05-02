// Realistic load profiles - Simulation of different user types
import http from "k6/http";
import { check, group, sleep } from "k6";
import { Counter, Trend, Rate } from "k6/metrics";
import { uuidv4 } from "https://jslib.k6.io/k6-utils/1.4.0/index.js";

const base = __ENV.API_BASE || "http://127.0.0.1:8080";
const payer = __ENV.PAYER_ID || "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const payee = __ENV.PAYEE_ID || "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";

// Metrics segmented by user type
const retailPayments = new Counter("retail_payments_total");
const corporatePayments = new Counter("corporate_payments_total");
const retailLatency = new Trend("retail_payment_latency_ms", true);
const corporateLatency = new Trend("corporate_payment_latency_ms", true);

// Metrics by value range
const microPayments = new Counter("micro_payments_total");      // < $50
const standardPayments = new Counter("standard_payments_total"); // $50-500
const largePayments = new Counter("large_payments_total");       // $500-5k
const corporateHighValue = new Counter("corporate_high_value_total"); // > $5k

// User behavior metrics
const userSessions = new Counter("user_sessions_total");
const averageSessionLength = new Trend("session_length_seconds", true);
const paymentsPerSession = new Trend("payments_per_session", true);

// Temporal metrics
const businessHoursPayments = new Counter("business_hours_payments");
const afterHoursPayments = new Counter("after_hours_payments");
const weekendPayments = new Counter("weekend_payments");

export const options = {
  scenarios: {
    // Retail users - PIX and small transfers
    retail_users_pix: {
      executor: "ramping-vus",
      exec: "retail_pix_user",
      startVUs: 0,
      stages: [
        { duration: "2m", target: 10 },   // Morning - gradual growth
        { duration: "4m", target: 25 },   // Business hours peak
        { duration: "2m", target: 30 },   // Lunch - maximum peak
        { duration: "4m", target: 20 },   // Afternoon 
        { duration: "3m", target: 35 },   // End of business - second peak
        { duration: "3m", target: 15 },   // Evening
        { duration: "2m", target: 5 },    // Dawn
        { duration: "2m", target: 0 }
      ],
      gracefulRampDown: "30s"
    },

    // Corporate users - Higher values, less frequent
    corporate_users_ted: {
      executor: "ramping-vus",
      exec: "corporate_ted_user",
      startVUs: 0,
      stages: [
        { duration: "3m", target: 2 },    // Start of business
        { duration: "6m", target: 8 },    // Business hours
        { duration: "3m", target: 12 },   // Corporate peak
        { duration: "6m", target: 6 },    // Afternoon
        { duration: "4m", target: 2 },    // End of business
        { duration: "2m", target: 0 }     // After hours
      ],
      gracefulRampDown: "1m"
    },

    // E-commerce - Online shopping pattern
    ecommerce_pattern: {
      executor: "ramping-vus",
      exec: "ecommerce_checkout_user", 
      startVUs: 0,
      stages: [
        { duration: "1m", target: 5 },    // Low morning
        { duration: "3m", target: 15 },   // Growth
        { duration: "2m", target: 25 },   // Lunch peak
        { duration: "2m", target: 20 },   // Afternoon
        { duration: "4m", target: 40 },   // Evening - e-commerce peak
        { duration: "2m", target: 15 },   // Late evening
        { duration: "1m", target: 0 }
      ],
      gracefulRampDown: "30s"
    },

    // Black Friday / Special events
    black_friday_simulation: {
      executor: "ramping-vus",
      exec: "black_friday_user",
      startVUs: 0,
      stages: [
        { duration: "30s", target: 10 },  // Preparation
        { duration: "1m", target: 50 },   // Promotions start
        { duration: "2m", target: 100 },  // Black Friday peak
        { duration: "1m", target: 80 },   // Sustain
        { duration: "2m", target: 40 },   // Decline
        { duration: "1m", target: 0 }
      ],
      startTime: "15m", // Execute after other scenarios stabilize
      gracefulRampDown: "1m"
    },

    // Mobile users - Differentiated behavior
    mobile_app_users: {
      executor: "constant-vus",
      exec: "mobile_app_user",
      vus: Number(__ENV.MOBILE_VUS || 8),
      duration: "12m",
      gracefulStop: "30s"
    }
  },

  thresholds: {
    // Differentiated SLAs by user type
    "retail_payment_latency_ms": ["p(95)<2000"],        // Retail: P95 < 2s
    "corporate_payment_latency_ms": ["p(90)<3000"],     // Corporate: P90 < 3s
    
    // Volume targets
    "retail_payments_total": ["count>200"],             // At least 200 retail payments
    "corporate_payments_total": ["count>15"],           // At least 15 corporate payments (TED is low-frequency by design)
    
    // Value distribution 
    "micro_payments_total": ["count>50"],               // Many micro transactions
    "large_payments_total": ["count>10"],               // Some large transactions
    
    // Session behavior
    "session_length_seconds": ["p(90)<600"],            // 90% of sessions < 10min
    "payments_per_session": ["avg>1.5"],                // Average > 1.5 payments/session
    
    // Overall performance during peaks
    "http_req_duration{scenario:black_friday_simulation}": ["p(95)<5000"],
    "http_req_failed{scenario:black_friday_simulation}": ["rate<0.15"]
  }
};

// Brazilian business hours simulation
function isBusinessHours() {
  const now = new Date();
  const hour = now.getHours();
  const day = now.getDay(); // 0 = Sunday, 6 = Saturday
  
  if (day === 0 || day === 6) {
    weekendPayments.add(1);
    return false;
  }
  
  if (hour >= 9 && hour <= 18) {
    businessHoursPayments.add(1);
    return true;
  } else {
    afterHoursPayments.add(1);
    return false;
  }
}

function createPaymentWithMetrics(amount, userType, description, sessionId) {
  const startTime = Date.now();
  
  const payload = {
    idempotency_key: uuidv4(),
    amount_cents: amount,
    currency: "BRL",
    payer_id: payer,
    payee_id: payee,
    description: description
  };

  const res = http.post(`${base}/payments`, JSON.stringify(payload), {
    headers: { 
      "Content-Type": "application/json",
      "X-Session-ID": sessionId,
      "X-User-Type": userType
    },
    tags: { 
      name: "realistic_payment",
      user_type: userType,
      amount_range: getAmountRange(amount),
      business_hours: isBusinessHours() ? "yes" : "no"
    }
  });

  const latency = Date.now() - startTime;

  // Metrics by user type
  if (userType === "retail") {
    retailPayments.add(1);
    retailLatency.add(latency);
  } else if (userType === "corporate") {
    corporatePayments.add(1);
    corporateLatency.add(latency);
  }

  // Metrics by value range
  if (amount < 5000) microPayments.add(1);
  else if (amount < 50000) standardPayments.add(1);
  else if (amount < 500000) largePayments.add(1);
  else corporateHighValue.add(1);

  return res;
}

function getAmountRange(amount) {
  if (amount < 5000) return "micro";       // < $50
  if (amount < 50000) return "standard";   // $50-500  
  if (amount < 500000) return "large";     // $500-5k
  return "corporate";                      // > $5k
}

export function retail_pix_user() {
  group("Retail PIX User Session", () => {
    const sessionId = uuidv4();
    const sessionStart = Date.now();
    userSessions.add(1);
    
    // Typical retail user - 1 to 4 PIX per session
    const transactionsInSession = 1 + Math.floor(Math.random() * 3);
    
    for (let i = 0; i < transactionsInSession; i++) {
      // PIX retail: $5 to $800 (concentration on low values)
      const amount = Math.floor(Math.random() < 0.7 ? 
        500 + Math.random() * 9500 :    // 70% between $5-100
        5000 + Math.random() * 75000    // 30% between $50-800
      );
      
      const descriptions = [
        "PIX to friend", "Restaurant bill split", "Market PIX",
        "Freelancer payment", "Uber split", "Family PIX"
      ];
      
      const res = createPaymentWithMetrics(
        amount, 
        "retail", 
        descriptions[Math.floor(Math.random() * descriptions.length)],
        sessionId
      );
      
      check(res, {
        "Retail PIX: Payment created": (r) => r.status === 202,
        "Retail PIX: Fast response": (r) => r.timings.duration < 2000
      });
      
      // Interval between PIX in the same session
      if (i < transactionsInSession - 1) {
        sleep(2 + Math.random() * 8); // 2-10s between transactions
      }
    }
    
    const sessionDuration = (Date.now() - sessionStart) / 1000;
    averageSessionLength.add(sessionDuration);
    paymentsPerSession.add(transactionsInSession);
    
    console.log(`Retail session: ${transactionsInSession} PIX in ${sessionDuration}s`);
  });
  
  // Time until next session - retail users are frequent
  sleep(30 + Math.random() * 120); // 30s to 2.5min
}

export function corporate_ted_user() {
  group("Corporate TED User Session", () => {
    const sessionId = uuidv4();
    const sessionStart = Date.now();
    userSessions.add(1);
    
    // Typical corporate - 1-2 transactions per session, higher values
    const transactionsInSession = Math.random() < 0.7 ? 1 : 2;
    
    for (let i = 0; i < transactionsInSession; i++) {
      // Corporate TED: $1,000 to $50,000
      const amount = Math.floor(
        100000 + Math.random() * 4900000 // $1k-50k
      );
      
      const descriptions = [
        "Supplier payment", "Payroll TED", "Service provider payment",
        "Head office transfer", "Corporate payment", "Corporate TED"
      ];
      
      const res = createPaymentWithMetrics(
        amount,
        "corporate", 
        descriptions[Math.floor(Math.random() * descriptions.length)],
        sessionId
      );
      
      check(res, {
        "Corporate TED: Payment created": (r) => r.status === 202,
        "Corporate TED: Acceptable latency": (r) => r.timings.duration < 3000,
        "Corporate TED: High value handled": () => amount >= 100000
      });
      
      // Longer interval between corporate TEDs
      if (i < transactionsInSession - 1) {
        sleep(30 + Math.random() * 180); // 30s-3min between transactions
      }
    }
    
    const sessionDuration = (Date.now() - sessionStart) / 1000;
    averageSessionLength.add(sessionDuration);
    paymentsPerSession.add(transactionsInSession);
  });
  
  // Corporate users have less frequent sessions
  sleep(300 + Math.random() * 900); // 5-20min until next session
}

export function ecommerce_checkout_user() {
  group("E-commerce Checkout Session", () => {
    const sessionId = uuidv4();
    userSessions.add(1);
    
    // E-commerce: usually 1 payment per session
    // Typical online shopping values: $25-500
    const amount = Math.floor(
      2500 + Math.random() * 47500  // $25-500
    );
    
    const products = [
      "E-commerce payment", "Online purchase", "Virtual store checkout",
      "Marketplace payment", "App purchase", "Delivery order"
    ];
    
    const res = createPaymentWithMetrics(
      amount,
      "ecommerce",
      products[Math.floor(Math.random() * products.length)],
      sessionId  
    );
    
    check(res, {
      "E-commerce: Checkout successful": (r) => r.status === 202,
      "E-commerce: Fast checkout": (r) => r.timings.duration < 1500,
      "E-commerce: Typical amount range": () => amount >= 2500 && amount <= 50000
    });
    
    paymentsPerSession.add(1);
    averageSessionLength.add(Math.random() * 60 + 30); // 30s-1.5min session
  });
  
  sleep(10 + Math.random() * 50); // 10s-1min between checkouts
}

export function black_friday_user() {
  group("Black Friday High-Load Session", () => {
    const sessionId = uuidv4();
    userSessions.add(1);
    
    // Black Friday: users make multiple purchases quickly
    const transactionsInSession = 1 + Math.floor(Math.random() * 4); // 1-4 purchases
    
    for (let i = 0; i < transactionsInSession; i++) {
      // Black Friday: varied values, tendency towards promotions (lower)
      const amount = Math.floor(Math.random() < 0.8 ?
        1000 + Math.random() * 19000 :   // 80% $10-200 (promotions)
        5000 + Math.random() * 95000     // 20% $50-1000 (expensive items)
      );
      
      const res = createPaymentWithMetrics(
        amount,
        "black_friday",
        `Black Friday purchase ${i+1}`,
        sessionId
      );
      
      check(res, {
        "Black Friday: System handles load": (r) => r.status === 202 || r.status === 429,
        "Black Friday: Response under pressure": (r) => r.timings.duration < 5000
      });
      
      // Short interval between purchases during Black Friday
      if (i < transactionsInSession - 1) {
        sleep(1 + Math.random() * 4); // 1-5s between purchases
      }
    }
    
    paymentsPerSession.add(transactionsInSession);
    console.log(`Black Friday user: ${transactionsInSession} purchases`);
  });
  
  sleep(5 + Math.random() * 15); // 5-20s until next Black Friday user
}

export function mobile_app_user() {
  group("Mobile App User Session", () => {
    const sessionId = uuidv4();
    userSessions.add(1);
    
    // Mobile: mixed behavior, short sessions
    const transactionsInSession = Math.random() < 0.6 ? 1 : 2;
    
    for (let i = 0; i < transactionsInSession; i++) {
      // Mobile: small to medium values
      const amount = Math.floor(
        500 + Math.random() * 29500  // $5-300
      );
      
      const res = createPaymentWithMetrics(
        amount,
        "mobile",
        "Mobile app payment",
        sessionId
      );
      
      check(res, {
        "Mobile: Payment successful": (r) => r.status === 202,
        "Mobile: Mobile-optimized latency": (r) => r.timings.duration < 2500
      });
      
      if (i < transactionsInSession - 1) {
        sleep(3 + Math.random() * 12); // 3-15s
      }
    }
    
    paymentsPerSession.add(transactionsInSession);
  });
  
  sleep(60 + Math.random() * 240); // 1-5min between mobile sessions
}