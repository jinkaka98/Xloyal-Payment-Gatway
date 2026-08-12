"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Activity, AlertTriangle, ArrowRight, Cable, CheckCircle2, Database, Globe2, Monitor, RefreshCw, Server, Waypoints } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { formatDate, formatRelativeTime } from "@/lib/format";
import type { HealthKind, ProviderHealth, SystemHealthSnapshot } from "@/lib/types";

const railIDs = ["frontend-console", "admin-proxy", "backend-api", "database"];
const kindLabels: Record<HealthKind, string> = {
  frontend: "Frontend",
  admin_proxy: "API proxy",
  backend_api: "Backend API",
  database: "Database",
  browser_session: "Browser session",
  provider_api: "Provider API",
};

function HealthIcon({ kind, size = 20 }: { kind: HealthKind; size?: number }) {
  if (kind === "frontend") return <Monitor size={size} />;
  if (kind === "admin_proxy") return <Waypoints size={size} />;
  if (kind === "backend_api") return <Server size={size} />;
  if (kind === "database") return <Database size={size} />;
  if (kind === "browser_session") return <Cable size={size} />;
  return <Globe2 size={size} />;
}

function displayTime(value?: string) {
  return value ? formatDate(value) : "Belum pernah";
}

export function SystemHealthMonitor() {
  const [snapshot, setSnapshot] = useState<SystemHealthSnapshot | null>(null);
  const [checking, setChecking] = useState(true);
  const [error, setError] = useState("");

  const runChecks = useCallback(async () => {
    setChecking(true);
    const started = performance.now();
    try {
      const response = await fetch("/api/system-health", { cache: "no-store" });
      if (!response.ok) throw new Error(`Health endpoint returned ${response.status}`);
      const next = await response.json() as SystemHealthSnapshot;
      const browserLatency = Math.round(performance.now() - started);
      next.checks = next.checks.map((item) => item.id === "frontend-console"
        ? { ...item, latencyMs: browserLatency, message: "Browser tersambung ke frontend dan health endpoint" }
        : item);
      setSnapshot(next);
      setError("");
    } catch {
      setError("Pemantauan frontend tidak menerima respons dari health endpoint.");
    } finally {
      setChecking(false);
    }
  }, []);

  useEffect(() => {
    void runChecks();
    const interval = window.setInterval(() => void runChecks(), 15000);
    return () => window.clearInterval(interval);
  }, [runChecks]);

  const rail = useMemo(() => railIDs.map((id) => snapshot?.checks.find((item) => item.id === id)).filter((item): item is ProviderHealth => Boolean(item)), [snapshot]);
  const passing = snapshot?.checks.filter((item) => item.status === "operational").length ?? 0;
  const total = snapshot?.checks.length ?? 0;
  const overall = error ? "offline" : snapshot?.overallStatus ?? "degraded";
  const healthy = overall === "operational";

  return <div className="page health-page">
    <PageHeader eyebrow="Pemantauan / Infrastruktur" title="System Health" description="Status langsung jalur frontend, backend, database, API provider, dan browser Merchant ID." actions={<button className="button" onClick={() => void runChecks()} disabled={checking}><RefreshCw size={17} className={checking ? "spin" : undefined} />{checking ? "Memeriksa" : "Periksa sekarang"}</button>} />

    <div className={`health-banner health-banner-${overall}`}>
      {healthy ? <CheckCircle2 size={24} /> : <AlertTriangle size={24} />}
      <div><strong>{healthy ? "Semua jalur koneksi tersambung" : "Sebagian jalur memerlukan perhatian"}</strong><p>{error || `${passing} dari ${total} pemeriksaan dalam kondisi operasional.`}</p></div>
      <span>{snapshot ? `Diperiksa ${formatRelativeTime(snapshot.checkedAt)}` : "Menunggu hasil"}</span>
    </div>

    <section className="health-path-section" aria-labelledby="connection-path-title">
      <div className="health-section-heading"><div><span className="section-kicker">Connection path</span><h2 id="connection-path-title">Jalur frontend ke backend</h2></div><span>Refresh otomatis 15 detik</span></div>
      <div className="health-path">
        {rail.map((item, index) => <div className="health-path-fragment" key={item.id}>
          <article className={`health-path-node health-path-${item.status}`}>
            <div className="health-path-icon"><HealthIcon kind={item.kind} size={18} /></div>
            <div><span>{kindLabels[item.kind]}</span><strong>{item.name}</strong><small>{item.latencyMs} ms</small></div>
            <StatusBadge status={item.status} />
          </article>
          {index < rail.length - 1 ? <ArrowRight className="health-path-arrow" size={18} aria-hidden="true" /> : null}
        </div>)}
      </div>
    </section>

    <div className="health-section-heading health-check-heading"><div><span className="section-kicker">Live checks</span><h2>Seluruh koneksi sistem</h2></div><span>{total} endpoint dan session dipantau</span></div>
    {snapshot ? <section className="health-grid" aria-label="Daftar pemeriksaan kesehatan">
      {snapshot.checks.map((item) => <article className="health-item" key={item.id}>
        <div className="health-item-top"><div className="health-icon"><HealthIcon kind={item.kind} /></div><StatusBadge status={item.status} /></div>
        <span className="health-kind">{kindLabels[item.kind]}</span>
        <h2>{item.name}</h2>
        <p>{item.message}</p>
        <dl>
          <div><dt>Endpoint</dt><dd title={item.endpoint}>{item.endpoint || "Internal"}</dd></div>
          <div><dt>Latency terakhir</dt><dd>{item.latencyMs} ms</dd></div>
          <div><dt>Terakhir tersambung</dt><dd>{displayTime(item.lastConnectedAt)}</dd></div>
          {item.kind === "browser_session" ? <div><dt>Sync transaksi terakhir</dt><dd>{displayTime(item.lastSyncedAt)}</dd></div> : null}
          <div><dt>Pemeriksaan terakhir</dt><dd>{formatRelativeTime(item.lastCheckedAt)}</dd></div>
        </dl>
      </article>)}
    </section> : <section className="health-loading"><Activity size={22} /><strong>Menghubungi seluruh service</strong><span>Memeriksa frontend, API, database, dan session merchant.</span></section>}
  </div>;
}
