// Duplicidade de negócio: o contador idem_replay_different_payment_id > 0 indica bug
// (mesma idempotency_key retornou payment_id diferente). Mesmo payment_id em replay é esperado.
import http from "k6/http";
import { check, sleep } from "k6";
import { Counter, Trend } from "k6/metrics";
import { uuidv4 } from "https://jslib.k6.io/k6-utils/1.4.0/index.js";

const duplicateIdemDifferentPayment = new Counter("idem_replay_different_payment_id");
const createLatency = new Trend("payment_create_duration_ms", true);

const base = __ENV.API_BASE || "http://127.0.0.1:8080";
const payer = __ENV.PAYER_ID || "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const payee = __ENV.PAYEE_ID || "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";

export const options = {
  scenarios: {
    steady_create: {
      executor: "constant-vus",
      exec: "steady_create",
      vus: Number(__ENV.VUS || 3),
      duration: __ENV.DURATION || "8s",
      gracefulStop: "3s",
    },
    idempotency_probe: {
      executor: "shared-iterations",
      exec: "idempotency_probe",
      vus: 1,
      iterations: Number(__ENV.IDEM_ITERATIONS || 5),
      startTime: "2s",
      maxDuration: "30s",
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.15"],
    checks: ["rate>0.85"],
    idem_replay_different_payment_id: ["count==0"],
  },
};

function postPayment(idem, amountCents) {
  const body = JSON.stringify({
    idempotency_key: idem,
    amount_cents: amountCents,
    currency: "BRL",
    payer_id: payer,
    payee_id: payee,
    description: "k6 load",
  });
  const t0 = Date.now();
  const res = http.post(`${base}/payments`, body, {
    headers: { "Content-Type": "application/json" },
    tags: { name: "create_payment" },
  });
  createLatency.add(Date.now() - t0);
  return res;
}

export function steady_create() {
  const idem = uuidv4();
  const res = postPayment(idem, 100 + Math.floor(Math.random() * 9000));
  const ok = check(res, {
    "create 202": (r) => r.status === 202,
    "create has payment_id": (r) => {
      try {
        const j = JSON.parse(r.body);
        return typeof j.payment_id === "string" && j.payment_id.length > 0;
      } catch {
        return false;
      }
    },
  });
  if (!ok) {
    console.error(`steady_create fail status=${res.status} body=${String(res.body).slice(0, 200)}`);
  }
  sleep(0.05 + Math.random() * 0.25);
}

export function idempotency_probe() {
  const idem = uuidv4();
  const amount = 777;
  const r1 = postPayment(idem, amount);
  const ok1 = check(r1, { "idem first 202": (r) => r.status === 202 });
  if (!ok1) {
    console.error(`idempotency first fail ${r1.status} ${r1.body}`);
    return;
  }
  let pid1;
  try {
    pid1 = JSON.parse(r1.body).payment_id;
  } catch {
    return;
  }

  const r2 = postPayment(idem, amount + 1);
  const ok2 = check(r2, { "idem replay 202": (r) => r.status === 202 });
  if (!ok2) {
    console.error(`idempotency replay fail ${r2.status} ${r2.body}`);
    return;
  }
  let pid2;
  try {
    pid2 = JSON.parse(r2.body).payment_id;
  } catch {
    return;
  }

  const same = pid1 === pid2;
  check(r2, { "idem replay same payment_id": () => same });
  if (!same) {
    duplicateIdemDifferentPayment.add(1);
    console.error(`BUG idempotency: same key different payment_id a=${pid1} b=${pid2}`);
  }
  sleep(0.2);
}
