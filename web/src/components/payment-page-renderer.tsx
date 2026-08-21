"use client";

import { useEffect, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import {
  AlertTriangle,
  ArrowRight,
  Check,
  Clock,
  Copy,
  Download,
  Loader2,
  RotateCcw,
  ShieldCheck,
  XCircle,
} from "lucide-react";
import type { CSSProperties } from "react";
import type { PaymentThemeConfig, PublicPaymentSession } from "@/lib/payment-public";

export interface PaymentPageRendererProps {
  session: PublicPaymentSession;
  remainingSeconds: number;
  statusLabel: string;
  connected: boolean;
  cancelling: boolean;
  onCancel: () => void;
  onRedirect: (url: string) => void;
}

const SYSTEM_BRANDING = { display_name: "Xloyal Payment", tagline: "Pembayaran aman" };

function formatAmount(amount: number, currency: string) {
  try { return new Intl.NumberFormat("id-ID", { style: "currency", currency, maximumFractionDigits: 0 }).format(amount); }
  catch { return `${currency} ${amount.toLocaleString("id-ID")}`; }
}

function formatExpiryParts(expiresAt?: string) {
  if (!expiresAt) return null;
  try {
    const d = new Date(expiresAt);
    if (isNaN(d.getTime())) return null;
    const formatted = new Intl.DateTimeFormat("id-ID", {
      weekday: "long",
      day: "numeric",
      month: "long",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
      timeZone: "Asia/Jakarta",
    }).format(d);
    return `${formatted.replace(/:/g, ".")} WIB`;
  } catch {
    return null;
  }
}

function colors(theme: PaymentThemeConfig) {
  return {
    accent: theme.colors?.primary ?? "#1a5c55",
    background: theme.colors?.background ?? "#f4f6f2",
    surface: theme.colors?.surface ?? "#ffffff",
    text: theme.colors?.text ?? "#18231f",
    muted: theme.colors?.muted ?? "#68766f",
    success: theme.colors?.success ?? "#16805b",
    danger: theme.colors?.danger ?? "#b44444",
  };
}

export function PaymentPageRenderer({ session, remainingSeconds = 0, statusLabel = "Menunggu pembayaran", cancelling, onCancel, onRedirect }: PaymentPageRendererProps) {
  const [copiedAmount, setCopiedAmount] = useState(false);
  const theme = session.theme?.config ?? {};
  const branding = { ...SYSTEM_BRANDING, ...(theme.tenant_branding ?? {}), ...(theme.branding ?? {}) };
  const palette = colors({ ...theme, colors: { ...theme.colors, primary: theme.colors?.primary ?? branding.primary_color ?? "#1a5c55" } });
  const template = theme.template_key ?? "modern";
  const isDark = template === "dark" || template === "fintech" || template === "midnight_gold";
  const isCorporate = template === "corporate";
  const isCompact = template === "compact";
  const isMinimal = template === "minimal";
  const layout = isMinimal ? "minimal" : isDark ? "dark" : isCorporate ? "corporate" : isCompact ? "compact" : "modern";
  const density = theme.layout?.density ?? (isCompact ? "compact" : "comfortable");
  const maxWidth = theme.layout?.max_width ?? (isCorporate ? 720 : isCompact ? 400 : isMinimal ? 460 : 520);
  const radius = theme.layout?.radius ?? (isMinimal ? 0 : isCorporate ? 8 : isCompact ? 6 : 14);
  const redirect = session.redirect;
  const status = session.status;
  const hasQR = Boolean(session.qr_payload) && (theme.payment_visibility?.show_qr ?? true);
  const timerEnabled = theme.timer?.enabled ?? true;
  const minutes = Math.floor(Math.max(0, remainingSeconds) / 60).toString().padStart(2, "0");
  const seconds = (Math.max(0, remainingSeconds) % 60).toString().padStart(2, "0");
  const isWaiting = status === "payment_pending";
  const isVerifying = statusLabel.toLowerCase().includes("verifikasi") || statusLabel.toLowerCase().includes("proses");
  const expiryFormatted = formatExpiryParts(session.expires_at);

  const [redirectCount, setRedirectCount] = useState(theme.redirect_delay ?? 5);

  useEffect(() => {
    if (status === "redirecting") {
      setRedirectCount(theme.redirect_delay ?? 5);
      const interval = setInterval(() => {
        setRedirectCount((prev) => {
          if (prev <= 1) {
            clearInterval(interval);
            if (redirect?.success_url) {
              onRedirect(redirect.success_url);
            }
            return 0;
          }
          return prev - 1;
        });
      }, 1000);
      return () => clearInterval(interval);
    }
  }, [status, theme.redirect_delay, redirect, onRedirect]);

  const dynamicStyles = {
    "--host-accent": palette.accent,
    "--host-bg": palette.background,
    "--host-surface": palette.surface,
    "--host-text": palette.text,
    "--host-muted": palette.muted,
    "--host-success": palette.success,
    "--host-danger": palette.danger,
    "--host-radius": `${radius}px`,
    "--host-max-width": `${maxWidth}px`,
  } as CSSProperties;

  const handleCopyAmount = async () => {
    try {
      await navigator.clipboard.writeText(session.amount.toString());
      setCopiedAmount(true);
      setTimeout(() => setCopiedAmount(false), 2000);
    } catch {
      // Fallback
    }
  };

  const handleDownloadQR = () => {
    const svg = document.getElementById(`qris-svg-${session.invoice_id || "payment"}`) as SVGSVGElement | null;
    if (!svg) return;
    const svgData = new XMLSerializer().serializeToString(svg);
    const canvas = document.createElement("canvas");
    const ctx = canvas.getContext("2d");
    const img = new Image();
    img.onload = () => {
      const scale = 2;
      canvas.width = (img.width || 300) * scale;
      canvas.height = (img.height || 300) * scale;
      if (ctx) {
        ctx.fillStyle = "#ffffff";
        ctx.fillRect(0, 0, canvas.width, canvas.height);
        ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
        const pngFile = canvas.toDataURL("image/png");
        const downloadLink = document.createElement("a");
        downloadLink.download = `QRIS-${session.invoice_id || "payment"}.png`;
        downloadLink.href = pngFile;
        downloadLink.click();
      }
    };
    img.src = `data:image/svg+xml;base64,${btoa(unescape(encodeURIComponent(svgData)))}`;
  };

  return (
    <main className={`hosted-page hosted-layout-${layout} hosted-theme-${template} hosted-density-${density}`} data-template={template} data-density={density} style={dynamicStyles}>
      <div className="hosted-shell">
        <header className="hosted-header">
          <div className="hosted-brand">
            {branding.logo_url ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={branding.logo_url}
                alt={branding.display_name ?? "Logo Merchant"}
                className="hosted-logo-full"
              />
            ) : (
              <>
                <span className="hosted-logo-fallback" aria-hidden="true">
                  {(branding.display_name ?? "X").charAt(0).toUpperCase()}
                </span>
                <div className="hosted-brand-info">
                  <strong>{branding.display_name ?? "Pembayaran"}</strong>
                  {branding.tagline && <span>{branding.tagline}</span>}
                </div>
              </>
            )}
          </div>
        </header>

        <section className="hosted-panel" aria-live="polite">
          <div className="hosted-panel-body">
            <div className="hosted-panel-main">
              {isWaiting && (
                <>
                  <div className={`hosted-status ${isVerifying ? "is-verifying" : ""}`}>
                    {isVerifying ? (
                      <Loader2 size={16} className="hosted-spin" />
                    ) : (
                      <span className="hosted-status-dot status-payment_pending" aria-hidden="true" />
                    )}
                    <span>{statusLabel}</span>
                  </div>
                  <p className="hosted-message">Pastikan Pembayaran sesuai Nominal</p>

                  {session.description && (theme.payment_visibility?.show_description ?? true) && (
                    <p className="hosted-description">{session.description}</p>
                  )}

                  {(theme.payment_visibility?.show_amount ?? true) && (
                    <>
                    {session.qris_merchant_name && <div className="hosted-merchant-identity">
                      <span className="hosted-merchant-label">Nama penerima QRIS</span>
                      <strong>{session.qris_merchant_name}</strong>
                      {session.qris_merchant_city && <small>{session.qris_merchant_city}</small>}
                      <span className="hosted-merchant-hint">Pastikan nama ini tampil setelah QR dipindai.</span>
                    </div>}
                    <div className="hosted-amount-row">
                      <span className="hosted-amount-value">{formatAmount(session.amount, session.currency)}</span>
                      <button
                        type="button"
                        className={`hosted-copy-btn ${copiedAmount ? "copied" : ""}`}
                        onClick={() => void handleCopyAmount()}
                        title="Salin nominal pembayaran"
                        aria-label="Salin nominal pembayaran"
                      >
                        {copiedAmount ? <Check size={14} /> : <Copy size={14} />}
                        {copiedAmount && <span>Tersalin</span>}
                      </button>
                    </div>
                    {session.unique_amount_code ? <div className="hosted-amount-breakdown">
                      <span>Nominal pesanan: {formatAmount(session.requested_amount ?? session.amount - session.unique_amount_code, session.currency)}</span>
                      <strong>Kode unik: {String(session.unique_amount_code).padStart(2, "0")}</strong>
                    </div> : null}
                    </>
                  )}

                  {timerEnabled && (
                    <div className={`hosted-expiry ${remainingSeconds < (theme.timer?.warning_seconds ?? 120) ? "is-warning" : ""}`}>
                      <span className="hosted-duration-label">Duration :</span>
                      <strong className="hosted-duration-timer">{minutes}:{seconds}</strong>
                    </div>
                  )}
                </>
              )}

              {status === "paid" && (
                <div className="hosted-result-box hosted-result-success">
                  <div className="hosted-success-anim-wrap">
                    <div className="hosted-success-ring" />
                    <div className="hosted-success-circle">
                      <Check size={36} strokeWidth={3} />
                    </div>
                  </div>
                  <h2 className="hosted-result-title">{theme.success_copy?.title ?? "Pembayaran Berhasil"}</h2>
                  <p className="hosted-result-desc">
                    {theme.success_copy?.message ?? "Terima kasih, pembayaran Anda telah berhasil kami terima dan diverifikasi."}
                  </p>
                  <div className="hosted-result-amount-badge">
                    <span>Total Terbayar:</span>
                    <strong>{formatAmount(session.amount, session.currency)}</strong>
                  </div>
                </div>
              )}

              {status === "redirecting" && (
                <div className="hosted-result-box hosted-result-redirecting">
                  <div className="hosted-success-anim-wrap">
                    <div className="hosted-success-ring" />
                    <div className="hosted-success-circle">
                      <Check size={34} strokeWidth={3} />
                    </div>
                  </div>
                  <h2 className="hosted-result-title">{theme.success_copy?.title ?? "Pembayaran Berhasil"}</h2>
                  <p className="hosted-result-desc">Mengalihkan kembali ke website merchant secara otomatis...</p>

                  <div className="hosted-redirect-countdown-card">
                    <div className="hosted-redirect-dial">
                      <span className="hosted-redirect-num">{redirectCount}</span>
                    </div>
                    <span className="hosted-redirect-caption">Mengalihkan dalam {redirectCount} detik</span>
                  </div>

                  {redirect?.success_url && (
                    <button
                      type="button"
                      className="hosted-btn-redirect-now"
                      onClick={() => onRedirect(redirect.success_url!)}
                    >
                      Lanjutkan ke Merchant Sekarang <ArrowRight size={15} />
                    </button>
                  )}
                </div>
              )}

              {/* STATUS FAILED */}
              {status === "failed" && (
                <div className="hosted-result-box hosted-result-failed">
                  <div className="hosted-failed-anim-wrap">
                    <div className="hosted-failed-circle">
                      <XCircle size={44} />
                    </div>
                  </div>
                  <h2 className="hosted-result-title">Pembayaran Gagal</h2>
                  <p className="hosted-result-desc">
                    Transaksi pembayaran tidak dapat diproses atau ditolak oleh sistem perbankan.
                  </p>
                  {redirect?.failed_url && (
                    <button
                      type="button"
                      className="hosted-btn-action-failed"
                      onClick={() => onRedirect(redirect.failed_url!)}
                    >
                      <RotateCcw size={14} /> Coba Bayar Ulang
                    </button>
                  )}
                </div>
              )}

              {/* STATUS EXPIRED */}
              {status === "expired" && (
                <div className="hosted-result-box hosted-result-expired">
                  <div className="hosted-expired-anim-wrap">
                    <div className="hosted-expired-circle">
                      <Clock size={40} />
                    </div>
                  </div>
                  <h2 className="hosted-result-title">Waktu Pembayaran Kedaluwarsa</h2>
                  <p className="hosted-result-desc">
                    Batas waktu sesi checkout ini telah berakhir. Silakan lakukan pemesanan ulang untuk mendapatkan kode QR baru.
                  </p>
                  {redirect?.expired_url && (
                    <button
                      type="button"
                      className="hosted-btn-action-expired"
                      onClick={() => onRedirect(redirect.expired_url!)}
                    >
                      <RotateCcw size={14} /> Buat Pesanan Baru
                    </button>
                  )}
                </div>
              )}

              {/* STATUS CANCELLED */}
              {status === "cancelled" && (
                <div className="hosted-result-box hosted-result-cancelled">
                  <div className="hosted-cancelled-anim-wrap">
                    <div className="hosted-cancelled-circle">
                      <AlertTriangle size={38} />
                    </div>
                  </div>
                  <h2 className="hosted-result-title">Pembayaran Dibatalkan</h2>
                  <p className="hosted-result-desc">
                    Pesanan telah dibatalkan oleh pengguna. Tagihan ini tidak lagi aktif.
                  </p>
                  {redirect?.cancel_url && (
                    <button
                      type="button"
                      className="hosted-btn-action-cancelled"
                      onClick={() => onRedirect(redirect.cancel_url!)}
                    >
                      Kembali ke Merchant
                    </button>
                  )}
                </div>
              )}

              {session.invoice_id && isWaiting && (
                <div className="hosted-invoice-tag">
                  <span>No. Faktur:</span>
                  <code>{session.invoice_id}</code>
                </div>
              )}
            </div>

            {/* SIDE PANEL (BARCODE & EXPIRATION DETAILS & ACTION BUTTONS) */}
            {isWaiting && (
              <div className="hosted-panel-side">
                {hasQR ? (
                  <div className={`hosted-qr ${isVerifying ? "is-verifying-scan" : ""}`}>
                    {isVerifying && <div className="hosted-scan-laser-line" />}
                    <QRCodeSVG
                      id={`qris-svg-${session.invoice_id || "payment"}`}
                      value={session.qr_payload!}
                      size={264}
                      bgColor="#ffffff"
                      fgColor="#101916"
                      level="M"
                      includeMargin
                      aria-label="QR pembayaran"
                    />
                  </div>
                ) : (
                  <div className="hosted-qr hosted-qr-missing">QR pembayaran belum tersedia</div>
                )}

                {/* BATAS WAKTU DI BAWAH BARCODE (JARAK ATAS & BAWAH) */}
                {expiryFormatted && (
                  <div className="hosted-expiry-under-qr">
                    <span className="hosted-expiry-label">Batas waktu:</span>
                    <span className="hosted-expiry-val">{expiryFormatted}</span>
                  </div>
                )}

                <div className="hosted-actions-stack">
                  {hasQR && (
                    <button
                      type="button"
                      className="hosted-btn-save"
                      onClick={handleDownloadQR}
                      title="Simpan gambar barcode ke perangkat"
                    >
                      <Download size={15} />
                      save img to device
                    </button>
                  )}
                  <button
                    type="button"
                    className="hosted-cancel"
                    onClick={onCancel}
                    disabled={cancelling}
                  >
                    {cancelling ? "Membatalkan..." : "Cancle Order"}
                  </button>
                </div>
              </div>
            )}
          </div>
        </section>
        <footer className="hosted-footer">
          <ShieldCheck size={13} style={{ verticalAlign: "-2px", marginRight: "4px" }} />
          Pembayaran aman & terverifikasi oleh alpakyros.com
        </footer>
      </div>
    </main>
  );
}
