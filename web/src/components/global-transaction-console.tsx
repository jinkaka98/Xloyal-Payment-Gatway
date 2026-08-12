"use client";

import { ChevronLeft, ChevronRight, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { formatCurrency, formatDate } from "@/lib/format";
import type { GlobalTransactionLog } from "@/lib/types";
import { EmptyState } from "./empty-state";
import { StatusBadge } from "./status-badge";

const pageSizes = [10, 20, 50, 100, 200];
const autoReloadOptions = [0, 1, 2, 5, 10];

function validationLabel(value: string) {
  const labels: Record<string, string> = {
    browser_sync_queued: "Sync browser dijadwalkan",
    merchant_connection_unavailable: "Koneksi merchant belum tersedia",
    no_matching_merchant_transaction: "Transaksi belum ditemukan",
    merchant_not_linked: "Merchant belum terhubung",
    ambiguous_amount_time: "Lebih dari satu transaksi cocok",
    amount_time_unique: "Nominal dan waktu cocok unik",
    qris_test_amount_time_unique: "Validasi QRIS Test cocok unik",
    expired_no_match: "Batas waktu habis tanpa transaksi cocok",
    unmatched: "Belum terhubung ke request",
  };
  return labels[value] ?? value.replaceAll("_", " ");
}

function paginationItems(page: number, total: number) {
  const pages = new Set([1, total, page - 1, page, page + 1]);
  const values = [...pages].filter((value) => value >= 1 && value <= total).sort((a, b) => a - b);
  const result: Array<number | string> = [];
  values.forEach((value, index) => {
    const previous = values[index - 1];
    if (previous !== undefined && value - previous > 1) result.push(`ellipsis-${value}`);
    result.push(value);
  });
  return result;
}

export function GlobalTransactionConsole({ initialItems }: { initialItems: GlobalTransactionLog[] }) {
  const [items, setItems] = useState(initialItems);
  const [pageSize, setPageSize] = useState(20);
  const [page, setPage] = useState(1);
  const [autoReload, setAutoReload] = useState(1);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [lastUpdated, setLastUpdated] = useState(() => new Date());

  const reload = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await fetch("/api/admin/global-transactions?limit=500", { cache: "no-store" });
      const payload = await response.json();
      if (!response.ok || !Array.isArray(payload)) throw new Error(payload?.error ?? `Request failed (${response.status})`);
      setItems(payload as GlobalTransactionLog[]);
      setLastUpdated(new Date());
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Data tidak dapat diperbarui");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (autoReload === 0) return;
    const timer = window.setInterval(() => void reload(), autoReload * 60_000);
    return () => window.clearInterval(timer);
  }, [autoReload, reload]);

  const totalPages = Math.max(1, Math.ceil(items.length / pageSize));
  useEffect(() => setPage((current) => Math.min(current, totalPages)), [totalPages]);
  const visible = useMemo(() => items.slice((page - 1) * pageSize, page * pageSize), [items, page, pageSize]);

  return <section className="section-block global-log-console">
    <div className="global-log-toolbar">
      <div><strong>{items.length} event</strong><span>Diperbarui {lastUpdated.toLocaleTimeString("id-ID", { hour: "2-digit", minute: "2-digit", second: "2-digit" })}</span></div>
      <div className="global-log-controls">
        <label><span>Auto reload</span><select value={autoReload} onChange={(event) => setAutoReload(Number(event.target.value))}>{autoReloadOptions.map((minutes) => <option key={minutes} value={minutes}>{minutes === 0 ? "Nonaktif" : `${minutes} menit`}</option>)}</select></label>
        <label><span>Baris</span><select value={pageSize} onChange={(event) => { setPageSize(Number(event.target.value)); setPage(1); }}>{pageSizes.map((size) => <option key={size} value={size}>{size}</option>)}</select></label>
        <button className="button" onClick={() => void reload()} disabled={loading}><RefreshCw size={16} className={loading ? "spin" : undefined} />{loading ? "Memuat" : "Reload"}</button>
      </div>
    </div>
    {error && <p className="global-log-error" role="alert">{error}</p>}
    {items.length === 0 ? <EmptyState title="Belum ada event transaksi" description="Request QRIS Test dan transaksi merchant akan muncul di sini." /> : <>
      <div className="table-scroll"><table><thead><tr><th>Event</th><th>Asal request</th><th>Merchant / Tenant</th><th className="align-right">Nominal</th><th>Status</th><th>Jadwal validasi</th><th>Hasil validasi</th></tr></thead><tbody>{visible.map((item) => <tr key={item.id}>
        <td><span className={`log-origin ${item.event_type === "qris_test_check" ? "test" : "merchant"}`}>{item.event_type === "qris_test_check" ? "QRIS Test" : "Merchant"}</span><code className="cell-subtitle">{item.reference}</code></td>
        <td><strong>{item.event_type === "qris_test_check" ? "Admin Console" : "Machine Checker"}</strong><span className="cell-subtitle">{item.request_source}</span><span className="cell-subtitle">Request: {formatDate(item.event_at)}</span></td>
        <td><code>{item.merchant_id || "Belum terhubung"}</code><span className="cell-subtitle">{item.tenant_id || "Semua tenant"}</span></td>
        <td className="align-right amount-cell">{formatCurrency(item.amount)}</td>
        <td><StatusBadge status={item.status} /><span className="cell-subtitle">{item.check_count > 0 ? `${item.check_count} kali check` : "Belum dicek"}</span></td>
        <td><strong>{item.expires_at ? `Expired: ${formatDate(item.expires_at)}` : `Paid: ${formatDate(item.event_at)}`}</strong><span className="cell-subtitle">{item.last_checked_at ? `Check terakhir: ${formatDate(item.last_checked_at)}` : "Check terakhir: belum ada"}</span>{item.next_check_at && <span className="cell-subtitle">Check berikutnya: {formatDate(item.next_check_at)}</span>}</td>
        <td><strong>{validationLabel(item.validation)}</strong>{item.matched_transaction_id && <span className="cell-subtitle">Match: <code>{item.matched_transaction_id}</code></span>}{item.invoice_id && <span className="cell-subtitle">Invoice: <code>{item.invoice_id}</code></span>}</td>
      </tr>)}</tbody></table></div>
      <div className="table-pagination"><span>Menampilkan {(page - 1) * pageSize + 1}-{Math.min(page * pageSize, items.length)} dari {items.length}</span><div><button className="icon-button table-action" aria-label="Halaman sebelumnya" disabled={page === 1} onClick={() => setPage((current) => current - 1)}><ChevronLeft size={16} /></button>{paginationItems(page, totalPages).map((item) => typeof item === "number" ? <button key={item} className={`page-number ${item === page ? "active" : ""}`} onClick={() => setPage(item)}>{item}</button> : <span key={item}>...</span>)}<button className="icon-button table-action" aria-label="Halaman berikutnya" disabled={page === totalPages} onClick={() => setPage((current) => current + 1)}><ChevronRight size={16} /></button></div></div>
    </>}
  </section>;
}
