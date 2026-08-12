import fs from "node:fs/promises";

const HISTORY_URL = "https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php";
const TRANSACTION_URL = "https://merchant.qris.interactive.co.id/v2/m/proses.php?required=getTransactions";
const BASE_URL = (process.env.CAMOFOX_BROWSER_URL || "http://127.0.0.1:9377").replace(/\/$/, "");
const API_KEY = process.env.CAMOFOX_BROWSER_API_KEY || "";

async function request(path, method = "GET", body) {
  let response;
  try {
    response = await fetch(`${BASE_URL}${path}`, {
      method,
      headers: {
        Accept: "application/json",
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
        ...(API_KEY ? { Authorization: `Bearer ${API_KEY}` } : {}),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(120000),
    });
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(`${method} ${path} failed: ${message}`);
  }
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(payload.error || payload.message || `camofox-browser request failed (${response.status})`);
  return payload;
}

async function evaluate(tabId, userId, expression, extended = false) {
  const payload = await request(`/tabs/${encodeURIComponent(tabId)}/${extended ? "evaluate-extended" : "evaluate"}`, "POST", {
    userId,
    expression,
    ...(extended ? { timeout: 60000 } : {}),
  });
  if (payload.ok === false) throw new Error(payload.error || "browser evaluation failed");
  return payload.result;
}

async function waitForPage(tabId, userId) {
  await request(`/tabs/${encodeURIComponent(tabId)}/wait`, "POST", {
    userId,
    timeout: 30000,
    waitForNetwork: true,
  });
}

async function credentials() {
  const path = process.env.CAMOUFOX_CREDENTIAL_FILE || "";
  if (!path) throw new Error("portal session expired; CAMOUFOX_CREDENTIAL_FILE is not configured");
  const values = (await fs.readFile(path, "utf8")).split(/\r?\n/).map((value) => value.trim()).filter(Boolean);
  if (values.length < 2) throw new Error("credential file must contain email and password lines");
  return values;
}

function jakartaDate(value) {
  const parts = new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Jakarta", year: "numeric", month: "2-digit", day: "2-digit" }).formatToParts(value);
  const map = Object.fromEntries(parts.map((part) => [part.type, part.value]));
  return `${map.day}/${map.month}/${map.year}`;
}

async function main() {
  let rawInput = "";
  for await (const chunk of process.stdin) rawInput += chunk;
  const input = JSON.parse(rawInput);
  const userId = String(input.merchant_id || "interactive-browser");
  const cookies = Array.isArray(input.cookies) ? input.cookies : JSON.parse(input.cookies || "[]");
  await request("/health");
  // Create the persistent browser context independently from portal navigation.
  // On Windows, bootstrapping with a lightweight HTTPS page avoids coupling
  // profile startup to the portal's slow navigation and history request.
  const opened = await request("/tabs", "POST", { userId, sessionKey: "interactive-qris", url: "https://example.com" });
  const tabId = opened.tabId;

  try {
    await waitForPage(tabId, userId);
    if (cookies.length) {
      await request(`/sessions/${encodeURIComponent(userId)}/cookies`, "POST", { cookies, tabId });
    }
    await request(`/tabs/${encodeURIComponent(tabId)}/navigate`, "POST", { userId, url: HISTORY_URL });
    await waitForPage(tabId, userId);

    let state = await evaluate(tabId, userId, `({url:location.href,hasLogin:!!document.querySelector("#username")})`);
    if (state.hasLogin || /\/login(?:\/|\.php|$)/i.test(state.url || "")) {
      const [username, password] = await credentials();
      await request(`/tabs/${encodeURIComponent(tabId)}/type`, "POST", { userId, selector: "#username", text: username });
      await request(`/tabs/${encodeURIComponent(tabId)}/type`, "POST", { userId, selector: "#password", text: password });
      await request(`/tabs/${encodeURIComponent(tabId)}/click`, "POST", { userId, selector: "#submitBtn" });
      await new Promise((resolve) => setTimeout(resolve, 4000));
      await waitForPage(tabId, userId);
      state = await evaluate(tabId, userId, `({url:location.href,hasLogin:!!document.querySelector("#username"),text:document.body.innerText})`);
      if (state.hasLogin || /\/login(?:\/|\.php|$)/i.test(state.url || "")) {
        if (/akun anda tidak ditemukan|tidak aktif/i.test(state.text || "")) throw new Error("portal login rejected: account not found or inactive");
        throw new Error("portal login did not establish an authenticated session");
      }
      await request(`/tabs/${encodeURIComponent(tabId)}/navigate`, "POST", { userId, url: HISTORY_URL });
      await waitForPage(tabId, userId);
    }

    const now = new Date();
    const start = new Date(now.getTime() - 30 * 86400000);
    const range = `${jakartaDate(start)} - ${jakartaDate(now)}`;
    const payload = await evaluate(tabId, userId, `(async () => {
      const body = new URLSearchParams({draw:"1",start:"0",length:"300",range:${JSON.stringify(range)},item:"",item_search:"",status:"all",limit:"300",store:"0"});
      const response = await fetch(${JSON.stringify(TRANSACTION_URL)}, {method:"POST",headers:{"Content-Type":"application/x-www-form-urlencoded; charset=UTF-8","X-Requested-With":"XMLHttpRequest"},body});
      if (!response.ok) throw new Error("history request failed (" + response.status + ")");
      return response.json();
    })()`, true);

    const transactions = [];
    for (const row of payload?.data || []) {
      const reference = String(row.idtrans || "").trim();
      const amount = Number.parseInt(row.nominal1, 10);
      const match = String(row.tgl || "").match(/^(\d{2})\/(\d{2})\/(\d{4}) (\d{2}):(\d{2}):(\d{2})$/);
      if (!reference || !Number.isFinite(amount) || !match) continue;
      const paidAt = `${match[3]}-${match[2]}-${match[1]}T${match[4]}:${match[5]}:${match[6]}+07:00`;
      const statusText = String(row.status || "").replace(/<[^>]+>/g, "").trim().toLowerCase();
      transactions.push({ reference, amount, status: statusText === "sukses" ? "paid" : statusText || "unknown", paid_at: paidAt });
    }
    process.stdout.write(JSON.stringify({ transactions }));
  } finally {
    await request(`/tabs/${encodeURIComponent(tabId)}`, "DELETE", { userId }).catch(() => {});
  }
}

main().catch((error) => {
  process.stderr.write(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
});
