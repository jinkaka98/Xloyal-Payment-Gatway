import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { SESSION_COOKIE, verifySessionToken } from "@/lib/session";

const API_URL = process.env.INTERNAL_API_URL ?? "http://127.0.0.1:8080";
const idPattern = /^[a-f0-9]{32}$/;
const loopbackHosts = new Set(["localhost", "127.0.0.1", "::1"]);

async function authorize() {
  return verifySessionToken(
    (await cookies()).get(SESSION_COOKIE)?.value,
    process.env.CONSOLE_SESSION_SECRET ?? "",
  );
}

function adminPath(method: string, path: string[]) {
  const [resource, id, action] = path;
  if (path.length === 1 && resource === "templates" && (method === "GET" || method === "POST")) {
    return "/admin/qris-templates";
  }
  if (path.length === 3 && resource === "templates" && typeof id === "string" && idPattern.test(id) && action === "image" && method === "GET") {
    return `/admin/qris-templates/${id}/image`;
  }
  if (path.length === 1 && resource === "test-payments" && (method === "GET" || method === "POST")) {
    return "/admin/qris-test-payments";
  }
  if (path.length === 3 && resource === "test-payments" && typeof id === "string" && idPattern.test(id) && action === "qr" && method === "GET") {
    return `/admin/qris-test-payments/${id}/qr`;
  }
  return "";
}

function trustedOrigin(origin: string, requestURL: string) {
  try {
    const supplied = new URL(origin);
    const expected = new URL(requestURL);
    if (supplied.protocol !== expected.protocol || supplied.port !== expected.port) return false;
    return supplied.hostname === expected.hostname || (loopbackHosts.has(supplied.hostname) && loopbackHosts.has(expected.hostname));
  } catch {
    return false;
  }
}

async function proxy(request: Request, context: { params: Promise<{ path: string[] }> }) {
  if (!await authorize()) return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  const origin = request.headers.get("Origin");
  if (request.method === "POST" && origin && !trustedOrigin(origin, request.url)) {
    return NextResponse.json({ error: "invalid origin" }, { status: 403 });
  }
  const path = adminPath(request.method, (await context.params).path);
  if (!path) return NextResponse.json({ error: "not found" }, { status: 404 });
  const token = process.env.ADMIN_API_TOKEN;
  if (!token) return NextResponse.json({ error: "admin API is not configured" }, { status: 503 });

  const headers = new Headers({ Accept: request.headers.get("Accept") ?? "application/json", Authorization: `Bearer ${token}` });
  let body: BodyInit | undefined;
  if (request.method === "POST") {
    if (path === "/admin/qris-templates") {
      body = await request.formData();
    } else {
      headers.set("Content-Type", "application/json");
      body = await request.text();
    }
  }
  const response = await fetch(`${API_URL}${path}`, { method: request.method, headers, body: body ?? null, cache: "no-store" });
  return new NextResponse(response.body, {
    status: response.status,
    headers: { "Content-Type": response.headers.get("Content-Type") ?? "application/octet-stream", "Cache-Control": "no-store" },
  });
}

export const GET = proxy;
export const POST = proxy;
