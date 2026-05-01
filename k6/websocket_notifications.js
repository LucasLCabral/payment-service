// WebSocket tests for real-time status notifications
import http from "k6/http";
import ws from "k6/ws";
import { check, sleep } from "k6";
import { Counter, Trend } from "k6/metrics";
import { uuidv4 } from "https://jslib.k6.io/k6-utils/1.4.0/index.js";

const base = __ENV.API_BASE || "http://127.0.0.1:8080";
const wsBase = __ENV.WS_BASE || "ws://127.0.0.1:8080";
const payer = __ENV.PAYER_ID || "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const payee = __ENV.PAYEE_ID || "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";

// Custom metrics for WebSocket
const wsConnections = new Counter("websocket_connections_total");
const wsMessages = new Counter("websocket_messages_received");
const wsNotificationLatency = new Trend("websocket_notification_latency_ms", true);
const wsConnectionTime = new Trend("websocket_connection_time_ms", true);
const wsErrors = new Counter("websocket_errors_total");

export const options = {
  scenarios: {
    ws_notifications: {
      executor: "constant-vus",
      exec: "websocket_notification_test",
      vus: Number(__ENV.WS_VUS || 2),
      duration: __ENV.WS_DURATION || "30s",
      gracefulStop: "10s"
    },
    ws_stress: {
      executor: "ramping-vus", 
      exec: "websocket_stress_test",
      startVUs: 0,
      stages: [
        { duration: "10s", target: 5 },
        { duration: "20s", target: 10 },
        { duration: "10s", target: 0 }
      ],
      gracefulRampDown: "5s"
    }
  },
  thresholds: {
    websocket_connections_total: ["count>0"],
    websocket_messages_received: ["count>0"], 
    websocket_notification_latency_ms: ["p(95)<5000"], // Notifications in < 5s
    websocket_connection_time_ms: ["p(90)<1000"],      // Connection in < 1s
    websocket_errors_total: ["count<5"]                // Maximum 5 errors
  }
};

function createPayment() {
  const payload = {
    idempotency_key: uuidv4(),
    amount_cents: 1000 + Math.floor(Math.random() * 9000), // $10-100
    currency: "BRL",
    payer_id: payer,
    payee_id: payee,
    description: "ws test payment"
  };

  const res = http.post(`${base}/payments`, JSON.stringify(payload), {
    headers: { "Content-Type": "application/json" },
    tags: { name: "ws_create_payment" }
  });

  if (res.status === 202) {
    return JSON.parse(res.body);
  }
  return null;
}

export function websocket_notification_test() {
  // 1. Create a payment
  const payment = createPayment();
  if (!payment) {
    console.error("Failed to create payment for WebSocket test");
    return;
  }

  const paymentId = payment.payment_id;
  const paymentCreatedAt = Date.now();

  // 2. Connect to WebSocket to receive notifications
  const wsUrl = `${wsBase}/payments/${paymentId}/notifications`;
  const connectionStart = Date.now();

  const params = {
    tags: { name: "payment_notifications" }
  };

  const res = ws.connect(wsUrl, params, function (socket) {
    wsConnections.add(1);
    wsConnectionTime.add(Date.now() - connectionStart);

    let notificationReceived = false;
    let messageCount = 0;

    socket.on("open", () => {
      console.log(`WebSocket opened for payment ${paymentId}`);
      
      // Send ping to keep connection active
      socket.send(JSON.stringify({ type: "ping" }));
    });

    socket.on("message", (data) => {
      messageCount++;
      wsMessages.add(1);
      
      try {
        const message = JSON.parse(data);
        console.log(`WS message received for ${paymentId}:`, message);

        // Check if it's a status notification
        if (message.type === "payment_status_update") {
          notificationReceived = true;
          const notificationLatency = Date.now() - paymentCreatedAt;
          wsNotificationLatency.add(notificationLatency);

          const validNotification = check(message, {
            "notification has payment_id": (m) => m.payment_id === paymentId,
            "notification has valid status": (m) => ["PENDING", "SETTLED", "DECLINED"].includes(m.status),
            "notification has timestamp": (m) => m.timestamp && new Date(m.timestamp).getTime() > 0,
            "notification latency acceptable": () => notificationLatency < 10000 // < 10s
          });

          if (validNotification) {
            console.log(`✅ Valid notification received for ${paymentId} in ${notificationLatency}ms`);
          } else {
            wsErrors.add(1);
            console.error(`❌ Invalid notification for ${paymentId}:`, message);
          }

          // Close connection after receiving valid notification
          if (message.status === "SETTLED" || message.status === "DECLINED") {
            socket.close();
          }
        } else if (message.type === "pong") {
          // Response to ping - keep connection
          console.log(`Pong received for ${paymentId}`);
        }
      } catch (e) {
        wsErrors.add(1);
        console.error(`Error parsing WebSocket message for ${paymentId}:`, e);
      }
    });

    socket.on("error", (e) => {
      wsErrors.add(1);
      console.error(`WebSocket error for ${paymentId}:`, e);
    });

    socket.on("close", () => {
      console.log(`WebSocket closed for payment ${paymentId}. Messages received: ${messageCount}`);
      
      check(null, {
        "received at least one notification": () => notificationReceived,
        "received reasonable number of messages": () => messageCount > 0 && messageCount < 10
      });
    });

    // Safety timeout - close after 30s if no final notification received
    socket.setTimeout(() => {
      if (!notificationReceived) {
        wsErrors.add(1);
        console.error(`Timeout waiting for notification on ${paymentId}`);
      }
      socket.close();
    }, 30000);

    // Send periodic ping to maintain connection
    const pingInterval = socket.setInterval(() => {
      socket.send(JSON.stringify({ type: "ping", timestamp: Date.now() }));
    }, 10000);

    socket.on("close", () => {
      socket.clearInterval(pingInterval);
    });
  });

  if (!res) {
    wsErrors.add(1);
    console.error(`Failed to establish WebSocket connection for ${paymentId}`);
  }

  sleep(1); // Pause between connections
}

export function websocket_stress_test() {
  // Stress test - multiple simultaneous connections
  const payments = [];
  
  // Create multiple payments
  for (let i = 0; i < 3; i++) {
    const payment = createPayment();
    if (payment) {
      payments.push(payment);
    }
  }

  if (payments.length === 0) {
    console.error("No payments created for stress test");
    return;
  }

  // Connect to multiple WebSockets simultaneously
  payments.forEach((payment, index) => {
    const wsUrl = `${wsBase}/payments/${payment.payment_id}/notifications`;
    
    setTimeout(() => {
      const res = ws.connect(wsUrl, { tags: { name: "stress_test" } }, function (socket) {
        wsConnections.add(1);

        socket.on("open", () => {
          console.log(`Stress WS ${index} opened for ${payment.payment_id}`);
        });

        socket.on("message", (data) => {
          wsMessages.add(1);
          try {
            const message = JSON.parse(data);
            if (message.type === "payment_status_update") {
              console.log(`Stress WS ${index} received update:`, message.status);
              
              if (message.status === "SETTLED" || message.status === "DECLINED") {
                socket.close();
              }
            }
          } catch (e) {
            wsErrors.add(1);
          }
        });

        socket.on("error", (e) => {
          wsErrors.add(1);
          console.error(`Stress WS ${index} error:`, e);
        });

        // Auto-close after 20s
        socket.setTimeout(() => socket.close(), 20000);
      });

      if (!res) {
        wsErrors.add(1);
      }
    }, index * 100); // Stagger connections by 100ms
  });

  sleep(2);
}