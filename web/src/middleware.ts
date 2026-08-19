import { NextRequest, NextResponse } from "next/server";
import { SESSION_COOKIE, verifySessionToken } from "@/lib/session";

export async function middleware(request: NextRequest) {
  const valid = await verifySessionToken(
    request.cookies.get(SESSION_COOKIE)?.value,
    process.env.CONSOLE_SESSION_SECRET ?? "",
  );
  if (!valid) {
    const login = new URL("/login", request.url);
    login.searchParams.set("next", request.nextUrl.pathname);
    return NextResponse.redirect(login);
  }
  return NextResponse.next();
}

export const config = {
  matcher: ["/dashboard/:path*", "/tenants/:path*", "/merchant-accounts/:path*", "/merchant-ids/:path*", "/merchant-transactions/:path*", "/merchant-connecting/:path*", "/qris-control/:path*", "/global-transactions/:path*", "/invoices/:path*", "/qris-test/:path*", "/audit-logs/:path*", "/health/:path*", "/custom-web-payment/:path*"],
};
