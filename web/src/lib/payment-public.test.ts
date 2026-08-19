import { describe, expect, it } from "vitest";
import { paymentEventsURL, type PaymentEvent, type PublicPaymentSession } from "./payment-public";

describe("public payment contract", () => {
  it("builds a token-encoded SSE resume URL without credentials", () => {
    expect(paymentEventsURL("token / unsafe", 7)).toContain("/v1/payment-sessions/token%20%2F%20unsafe/events?after_sequence=7");
  });

  it("represents only public-safe snapshot and event fields", () => {
    const session: PublicPaymentSession = { session_id: "session", invoice_id: "invoice", status: "payment_pending", payment_status: "pending", amount: 1000, currency: "IDR", expires_at: "2026-08-19T12:00:00Z", server_now: "2026-08-19T11:59:00Z" };
    const event: PaymentEvent = { event_id: "event", payment_session_id: "session", invoice_id: "invoice", status: "paid", sequence: 2, occurred_at: "2026-08-19T12:00:00Z" };
    expect(session.amount).toBe(1000);
    expect(event.sequence).toBe(2);
    expect(JSON.stringify(session)).not.toContain("api_key");
  });
});
