import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { SESSION_COOKIE, verifySessionToken } from "@/lib/session";

const API_URL = process.env.INTERNAL_API_URL ?? "http://127.0.0.1:8080";
const merchantID = /^[A-Za-z0-9_-]{1,128}$/;

async function authorize() { return verifySessionToken((await cookies()).get(SESSION_COOKIE)?.value, process.env.CONSOLE_SESSION_SECRET ?? ""); }
function resolvePath(method: string, parts: string[]) {
  const [resource, id, connection, action] = parts;
  const getResources = new Set(["health", "qris-templates", "qris-test-payments", "merchant-accounts", "dashboard", "invoices", "audit-events", "merchant-transactions", "tenant-transactions", "global-transactions", "merchant-ids"]);
  if (parts.length === 1 && method === "GET" && resource && getResources.has(resource)) return `/admin/${resource}`;
  if (resource === "merchant-accounts" && parts.length === 2 && id && merchantID.test(id) && ["GET", "PUT"].includes(method)) return `/admin/merchant-accounts/${id}`;
  if (resource === "tenants" && parts.length === 1 && ["GET", "POST"].includes(method)) return "/admin/tenants";
  if (resource === "tenants" && parts.length === 2 && id && merchantID.test(id) && ["PUT", "DELETE"].includes(method)) return `/admin/tenants/${id}`;
  if (resource === "tenants" && parts.length === 3 && id && merchantID.test(id) && connection === "credentials" && method === "GET") return `/admin/tenants/${id}/credentials`;
  if (resource === "tenants" && parts.length === 4 && id && merchantID.test(id) && connection === "credentials" && action === "rotate" && method === "POST") return `/admin/tenants/${id}/credentials/rotate`;
  if (resource === "merchant-ids" && parts.length === 1 && method === "POST") return "/admin/merchant-ids";
  if (resource === "merchant-connections" && parts.length === 2 && id === "har" && method === "POST") return "/admin/merchant-connections/har";
  if (resource !== "merchant-ids" || !id || !merchantID.test(id)) return "";
  if (parts.length === 2 && method === "GET") return `/admin/merchant-ids/${id}`;
  if (parts.length === 3 && connection === "connection" && method === "GET") return `/admin/merchant-ids/${id}/connection`;
  if (parts.length === 4 && connection === "connection" && ["session", "har", "revoke", "manual-login"].includes(action ?? "") && method === "POST") return `/admin/merchant-ids/${id}/connection/${action}`;
  if (parts.length === 3 && connection === "sync" && method === "POST") return `/admin/merchant-ids/${id}/sync`;
  return "";
}
async function proxy(request: Request, context: { params: Promise<{ path: string[] }> }) {
  if (!await authorize()) return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  const path = resolvePath(request.method, (await context.params).path);
  if (!path) return NextResponse.json({ error: "not found" }, { status: 404 });
  const token = process.env.ADMIN_API_TOKEN;
  if (!token) return NextResponse.json({ error: "admin API is not configured" }, { status: 503 });
  const target = new URL(`${API_URL}${path}`);
  target.search = new URL(request.url).search;
  const response = await fetch(target, { method: request.method, headers: { Accept: "application/json", "Content-Type": "application/json", Authorization: `Bearer ${token}` }, body: ["POST", "PUT"].includes(request.method) ? await request.text() : null, cache: "no-store" });
  return new NextResponse(response.body, { status: response.status, headers: { "Content-Type": response.headers.get("Content-Type") ?? "application/json", "Cache-Control": "no-store" } });
}
export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const DELETE = proxy;
