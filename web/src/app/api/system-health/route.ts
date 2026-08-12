import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import type { ProviderHealth, ProviderStatus, SystemHealthSnapshot } from "@/lib/types";
import { SESSION_COOKIE, verifySessionToken } from "@/lib/session";

const API_URL = process.env.INTERNAL_API_URL ?? "http://127.0.0.1:8080";

interface BackendHealth {
  id: string;
  name: string;
  kind: ProviderHealth["kind"];
  status: ProviderStatus;
  latency_ms: number;
  endpoint?: string;
  last_checked_at: string;
  last_connected_at?: string;
  last_synced_at?: string;
  updated_at?: string;
  message: string;
}

async function timedFetch(url: string, init?: RequestInit) {
  const started = performance.now();
  try {
    const response = await fetch(url, { ...init, cache: "no-store", signal: AbortSignal.timeout(7000) });
    return { response, latencyMs: Math.round(performance.now() - started) };
  } catch {
    return { response: null, latencyMs: Math.round(performance.now() - started) };
  }
}

function fromBackend(item: BackendHealth): ProviderHealth {
  return {
    id: item.id,
    name: item.name,
    kind: item.kind,
    status: item.status,
    latencyMs: item.latency_ms,
    endpoint: item.endpoint,
    lastCheckedAt: item.last_checked_at,
    lastConnectedAt: item.last_connected_at,
    lastSyncedAt: item.last_synced_at,
    updatedAt: item.updated_at,
    message: item.message,
  };
}

export async function GET() {
  const session = (await cookies()).get(SESSION_COOKIE)?.value;
  if (!await verifySessionToken(session, process.env.CONSOLE_SESSION_SECRET ?? "")) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  const checkedAt = new Date().toISOString();
  const token = process.env.ADMIN_API_TOKEN;
  const [publicAPI, adminAPI] = await Promise.all([
    timedFetch(`${API_URL}/v1/health`, { headers: { Accept: "application/json" } }),
    token
      ? timedFetch(`${API_URL}/admin/health`, { headers: { Accept: "application/json", Authorization: `Bearer ${token}` } })
      : Promise.resolve({ response: null, latencyMs: 0 }),
  ]);

  const publicPayload = publicAPI.response?.ok ? await publicAPI.response.json().catch(() => null) as { status?: string } | null : null;
  const backendHealthy = publicAPI.response?.ok === true && publicPayload?.status === "ok";
  const adminPayload = adminAPI.response?.ok ? await adminAPI.response.json().catch(() => null) : null;
  const adminChecks = Array.isArray(adminPayload) ? (adminPayload as BackendHealth[]) : [];
  const proxyHealthy = adminAPI.response?.ok === true && adminChecks.length > 0;

  const checks: ProviderHealth[] = [
    {
      id: "frontend-console",
      name: "Frontend Console",
      kind: "frontend",
      status: "operational",
      latencyMs: 0,
      endpoint: "/health",
      lastCheckedAt: checkedAt,
      lastConnectedAt: checkedAt,
      message: "Runtime Next.js aktif dan dapat menjalankan pemeriksaan",
    },
    {
      id: "admin-proxy",
      name: "Admin API Proxy",
      kind: "admin_proxy",
      status: proxyHealthy ? "operational" : "offline",
      latencyMs: adminAPI.latencyMs,
      endpoint: "/api/system-health -> /admin/health",
      lastCheckedAt: checkedAt,
      lastConnectedAt: proxyHealthy ? checkedAt : undefined,
      message: proxyHealthy ? "Proxy frontend tersambung ke admin API" : token ? "Admin API tidak memberikan respons yang dapat dipakai" : "ADMIN_API_TOKEN belum dikonfigurasi",
    },
    {
      id: "backend-api",
      name: "Go API",
      kind: "backend_api",
      status: backendHealthy ? "operational" : "offline",
      latencyMs: publicAPI.latencyMs,
      endpoint: "/v1/health",
      lastCheckedAt: checkedAt,
      lastConnectedAt: backendHealthy ? checkedAt : undefined,
      message: backendHealthy ? "Public API backend dapat dijangkau" : "Public API backend tidak dapat dijangkau",
    },
    ...adminChecks.filter((item) => item.id !== "backend-api").map(fromBackend),
  ];
  const overallStatus: ProviderStatus = checks.some((item) => item.status === "offline")
    ? "offline"
    : checks.some((item) => item.status === "degraded") ? "degraded" : "operational";
  const snapshot: SystemHealthSnapshot = { overallStatus, checkedAt, checks };
  return NextResponse.json(snapshot, { headers: { "Cache-Control": "no-store" } });
}

export const dynamic = "force-dynamic";
