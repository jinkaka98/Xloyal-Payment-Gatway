(globalThis.TURBOPACK || (globalThis.TURBOPACK = [])).push(["chunks/[root-of-the-server]__6d833251._.js",
"[externals]/node:buffer [external] (node:buffer, cjs)", ((__turbopack_context__, module, exports) => {

const mod = __turbopack_context__.x("node:buffer", () => require("node:buffer"));

module.exports = mod;
}),
"[externals]/node:async_hooks [external] (node:async_hooks, cjs)", ((__turbopack_context__, module, exports) => {

const mod = __turbopack_context__.x("node:async_hooks", () => require("node:async_hooks"));

module.exports = mod;
}),
"[project]/src/lib/session.ts [middleware-edge] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "SESSION_COOKIE",
    ()=>SESSION_COOKIE,
    "createSessionToken",
    ()=>createSessionToken,
    "sessionCookieOptions",
    ()=>sessionCookieOptions,
    "verifySessionToken",
    ()=>verifySessionToken
]);
const SESSION_COOKIE = "xloyal_console_session";
const SESSION_TTL_SECONDS = 8 * 60 * 60;
function encode(value) {
    let binary = "";
    for (const byte of value)binary += String.fromCharCode(byte);
    return btoa(binary).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}
function decode(value) {
    const padded = value.replaceAll("-", "+").replaceAll("_", "/").padEnd(Math.ceil(value.length / 4) * 4, "=");
    const binary = atob(padded);
    return new Uint8Array(Array.from(binary, (character)=>character.charCodeAt(0)));
}
async function signingKey(secret) {
    return crypto.subtle.importKey("raw", new TextEncoder().encode(secret), {
        name: "HMAC",
        hash: "SHA-256"
    }, false, [
        "sign",
        "verify"
    ]);
}
async function signature(payload, secret) {
    const key = await signingKey(secret);
    return encode(new Uint8Array(await crypto.subtle.sign("HMAC", key, new TextEncoder().encode(payload))));
}
async function createSessionToken(secret) {
    const payload = encode(new TextEncoder().encode(JSON.stringify({
        exp: Math.floor(Date.now() / 1000) + SESSION_TTL_SECONDS
    })));
    return `${payload}.${await signature(payload, secret)}`;
}
async function verifySessionToken(token, secret) {
    if (!token || !secret) return false;
    const [payload, suppliedSignature, extra] = token.split(".");
    if (!payload || !suppliedSignature || extra) return false;
    try {
        const valid = await crypto.subtle.verify("HMAC", await signingKey(secret), decode(suppliedSignature), new TextEncoder().encode(payload));
        if (!valid) return false;
        const parsed = JSON.parse(new TextDecoder().decode(decode(payload)));
        return typeof parsed.exp === "number" && parsed.exp > Math.floor(Date.now() / 1000);
    } catch  {
        return false;
    }
}
const sessionCookieOptions = {
    httpOnly: true,
    secure: process.env.CONSOLE_COOKIE_SECURE === "true",
    sameSite: "strict",
    path: "/",
    maxAge: SESSION_TTL_SECONDS
};
}),
"[project]/src/middleware.ts [middleware-edge] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "config",
    ()=>config,
    "middleware",
    ()=>middleware
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f$next$2f$dist$2f$esm$2f$api$2f$server$2e$js__$5b$middleware$2d$edge$5d$__$28$ecmascript$29$__$3c$locals$3e$__ = __turbopack_context__.i("[project]/node_modules/next/dist/esm/api/server.js [middleware-edge] (ecmascript) <locals>");
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f$next$2f$dist$2f$esm$2f$server$2f$web$2f$exports$2f$index$2e$js__$5b$middleware$2d$edge$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/next/dist/esm/server/web/exports/index.js [middleware-edge] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$src$2f$lib$2f$session$2e$ts__$5b$middleware$2d$edge$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/src/lib/session.ts [middleware-edge] (ecmascript)");
;
;
async function middleware(request) {
    const valid = await (0, __TURBOPACK__imported__module__$5b$project$5d2f$src$2f$lib$2f$session$2e$ts__$5b$middleware$2d$edge$5d$__$28$ecmascript$29$__["verifySessionToken"])(request.cookies.get(__TURBOPACK__imported__module__$5b$project$5d2f$src$2f$lib$2f$session$2e$ts__$5b$middleware$2d$edge$5d$__$28$ecmascript$29$__["SESSION_COOKIE"])?.value, process.env.CONSOLE_SESSION_SECRET ?? "");
    if (!valid) {
        const login = new URL("/login", request.url);
        login.searchParams.set("next", request.nextUrl.pathname);
        return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f$next$2f$dist$2f$esm$2f$server$2f$web$2f$exports$2f$index$2e$js__$5b$middleware$2d$edge$5d$__$28$ecmascript$29$__["NextResponse"].redirect(login);
    }
    return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f$next$2f$dist$2f$esm$2f$server$2f$web$2f$exports$2f$index$2e$js__$5b$middleware$2d$edge$5d$__$28$ecmascript$29$__["NextResponse"].next();
}
const config = {
    matcher: [
        "/dashboard/:path*",
        "/tenants/:path*",
        "/merchant-accounts/:path*",
        "/merchant-ids/:path*",
        "/merchant-transactions/:path*",
        "/merchant-connecting/:path*",
        "/qris-control/:path*",
        "/global-transactions/:path*",
        "/invoices/:path*",
        "/qris-test/:path*",
        "/audit-logs/:path*",
        "/health/:path*"
    ]
};
}),
]);

//# sourceMappingURL=%5Broot-of-the-server%5D__6d833251._.js.map