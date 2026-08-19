"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useParams } from "next/navigation";
import { PaymentPageRenderer } from "@/components/payment-page-renderer";
import {
  cancelPaymentSession,
  getPaymentSession,
  paymentEventsURL,
  type PaymentEvent,
  type PaymentStatus,
  type PublicPaymentSession,
} from "@/lib/payment-public";

type LoadState = "INITIALIZING" | "READY" | "NOT_FOUND" | "ERROR";

function label(status: PaymentStatus) {
  return ({
    payment_pending: "Menunggu Pembayaran",
    paid: "Pembayaran Berhasil",
    failed: "Pembayaran Gagal",
    expired: "Pembayaran Kedaluwarsa",
    cancelled: "Pembayaran Dibatalkan",
    redirecting: "Mengalihkan",
    closed: "Pembayaran Ditutup",
  } satisfies Record<PaymentStatus, string>)[status];
}

export default function HostedPaymentPage() {
  const params = useParams<{ token: string }>();
  const token = params.token ?? "";

  const [session, setSession] = useState<PublicPaymentSession | null>(null);
  const [loadState, setLoadState] = useState<LoadState>("INITIALIZING");
  const [remaining, setRemaining] = useState(0);
  const [connected, setConnected] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [redirectAt, setRedirectAt] = useState<number | null>(null);
  const sourceRef = useRef<EventSource | null>(null);
  const reconnectRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectAttempt = useRef(0);
  const sequenceRef = useRef(0);
  const clockOffsetRef = useRef(0);

  const applySession = useCallback((next: PublicPaymentSession) => {
    setSession(next);
    setLoadState("READY");
    clockOffsetRef.current = Date.parse(next.server_now) - Date.now();
    setRemaining(
      Math.max(
        0,
        Math.ceil(
          (Date.parse(next.expires_at) - (Date.now() + clockOffsetRef.current)) / 1000,
        ),
      ),
    );
    if (next.status === "paid" && redirectAt === null) {
      setRedirectAt(
        Date.now() + ((next.theme?.config.redirect_delay ?? 5) * 1000),
      );
    }
  }, [redirectAt]);

  const recover = useCallback(async () => {
    try {
      applySession(await getPaymentSession(token));
    } catch (error) {
      if ((error as { status?: number }).status === 404) {
        setLoadState("NOT_FOUND");
      } else {
        setLoadState("ERROR");
      }
    }
  }, [applySession, token]);

  const connectRef = useRef<() => void>(() => {});
  const connect = useCallback(() => {
    sourceRef.current?.close();
    setConnected(false);
    const source = new EventSource(paymentEventsURL(token, sequenceRef.current));
    sourceRef.current = source;
    source.onopen = () => {
      reconnectAttempt.current = 0;
      setConnected(true);
    };
    source.onerror = () => {
      source.close();
      setConnected(false);
      void recover();
      const delay = Math.min(30000, 1000 * (2 ** reconnectAttempt.current++));
      reconnectRef.current = setTimeout(() => connectRef.current(), delay);
    };
    const handle = (event: MessageEvent<string>) => {
      try {
        const next = JSON.parse(event.data) as PaymentEvent;
        if (next.sequence <= sequenceRef.current) return;
        if (next.sequence > sequenceRef.current + 1) {
          void recover();
          return;
        }
        sequenceRef.current = next.sequence;
        setSession((current) =>
          current
            ? {
                ...current,
                status: next.status as PaymentStatus,
                payment_status:
                  next.status === "paid" ? "paid" : current.payment_status,
              }
            : current,
        );
        if (next.status === "paid") setRedirectAt(Date.now() + 5000);
      } catch {
        // ignore
      }
    };
    [
      "payment.pending",
      "payment.verifying",
      "payment.paid",
      "payment.failed",
      "payment.expired",
      "payment.cancelled",
      "payment.redirecting",
      "payment.closed",
    ].forEach((type) => source.addEventListener(type, handle));
  }, [recover, token]);
  connectRef.current = connect;

  useEffect(() => {
    void (async () => {
      try {
        const next = await getPaymentSession(token);
        applySession(next);
        if (next.status === "payment_pending") connect();
      } catch (error) {
        if ((error as { status?: number }).status === 404) {
          setLoadState("NOT_FOUND");
        } else {
          setLoadState("ERROR");
        }
      }
    })();
    return () => {
      sourceRef.current?.close();
      if (reconnectRef.current) clearTimeout(reconnectRef.current);
    };
  }, [applySession, connect, token]);

  useEffect(() => {
    if (!session || !["payment_pending"].includes(session.status)) return;
    const timer = setInterval(
      () =>
        setRemaining(
          Math.max(
            0,
            Math.ceil(
              (Date.parse(session.expires_at) -
                (Date.now() + clockOffsetRef.current)) /
                1000,
            ),
          ),
        ),
      1000,
    );
    return () => clearInterval(timer);
  }, [session]);

  // Theme publication is presentation state, so it is refreshed independently
  // from payment events. This lets an already-open checkout URL pick up the
  // latest published default without changing its token or payment state.
  const sessionStatus = session?.status;
  useEffect(() => {
    if (sessionStatus !== "payment_pending") return;
    let active = true;
    const refresh = async () => {
      try {
        const next = await getPaymentSession(token);
        if (active) applySession(next);
      } catch {
        // The SSE channel remains the status source; a transient theme refresh
        // failure should not interrupt an active checkout.
      }
    };
    const timer = setInterval(() => void refresh(), 3000);
    const onVisible = () => {
      if (document.visibilityState === "visible") void refresh();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      active = false;
      clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [applySession, sessionStatus, token]);

  useEffect(() => {
    if (redirectAt === null) return;
    const tick = setInterval(() => {
      if (Date.now() >= redirectAt) {
        clearInterval(tick);
        const url = session?.redirect?.success_url;
        if (url) {
          if (url.startsWith("#") || url.startsWith("http")) {
            window.location.assign(url);
          }
        }
      }
    }, 250);
    return () => clearInterval(tick);
  }, [redirectAt, session]);

  async function cancel() {
    setCancelling(true);
    sourceRef.current?.close();
    try {
      applySession(await cancelPaymentSession(token));
    } catch (error) {
      const body = (error as { body?: { session?: PublicPaymentSession } }).body;
      if (body?.session) applySession(body.session);
      else void recover();
    } finally {
      setCancelling(false);
    }
  }

  if (loadState === "INITIALIZING") {
    return (
      <main className="hosted-page">
        <div className="hosted-centered">
          <div className="hosted-loader" aria-label="Memuat pembayaran" />
          <p>Memuat pembayaran...</p>
        </div>
      </main>
    );
  }

  if (loadState === "NOT_FOUND") {
    return (
      <main className="hosted-page">
        <div className="hosted-centered">
          <h1>Pembayaran tidak ditemukan</h1>
          <p>Periksa kembali tautan pembayaran Anda.</p>
        </div>
      </main>
    );
  }

  if (loadState === "ERROR" || !session) {
    return (
      <main className="hosted-page">
        <div className="hosted-centered">
          <h1>Pembayaran tidak dapat dimuat</h1>
          <p>Periksa koneksi Anda lalu coba lagi.</p>
          <button className="hosted-return" onClick={() => void recover()}>
            Coba lagi
          </button>
        </div>
      </main>
    );
  }

  return (
    <PaymentPageRenderer
      session={session}
      remainingSeconds={remaining}
      statusLabel={label(session.status)}
      connected={connected}
      cancelling={cancelling}
      onCancel={() => void cancel()}
      onRedirect={(url) => {
        if (url) window.location.assign(url);
      }}
    />
  );
}
