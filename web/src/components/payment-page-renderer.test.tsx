import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { PaymentPageRenderer } from "./payment-page-renderer";
import type { PublicPaymentSession } from "@/lib/payment-public";

describe("PaymentPageRenderer QRIS identity", () => {
  it("shows the payable merchant name and unique amount breakdown", () => {
    const session: PublicPaymentSession = {
      session_id: "session",
      invoice_id: "invoice",
      status: "payment_pending",
      payment_status: "pending",
      requested_amount: 1000,
      amount: 1001,
      unique_amount_code: 1,
      qris_merchant_name: "XLOYAL MERCHANT",
      qris_merchant_city: "BANDAR LAMPUNG",
      currency: "IDR",
      qr_payload: "000201010212fixture",
      expires_at: new Date(Date.now() + 60_000).toISOString(),
      server_now: new Date().toISOString(),
    };

    render(<PaymentPageRenderer session={session} remainingSeconds={120} statusLabel="Menunggu pembayaran" connected={true} cancelling={false} onCancel={vi.fn()} onRedirect={vi.fn()} />);

    expect(screen.getByText("XLOYAL MERCHANT")).toBeInTheDocument();
    expect(screen.getByText(/Pastikan nama ini tampil/i)).toBeInTheDocument();
    expect(screen.getByText("Kode unik: 01")).toBeInTheDocument();
    expect(screen.getByText((_, element) => element?.textContent?.replace(/\u00a0/g, " ") === "Nominal pesanan: Rp 1.000")).toBeInTheDocument();
  });
});
