#!/usr/bin/env python3
"""Run a Webwright browser sync and emit the worker transaction contract."""

from __future__ import annotations

import contextlib
import json
import os
import sys
from pathlib import Path
from typing import Any

TOOL_ROOT = Path(__file__).resolve().parent / "Webwright-Clockbrowser"
sys.path.insert(0, str(TOOL_ROOT / "src"))

from webwright.environments.local_browser import LocalBrowserEnvironment


def fail(message: str) -> None:
    print(message, file=sys.stderr)
    raise SystemExit(1)


def load_request() -> dict[str, Any]:
    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        fail(f"invalid checker input JSON: {exc}")
    if not isinstance(payload, dict) or not payload.get("merchant_id"):
        fail("checker input requires merchant_id")
    return payload


def browser_credential(value: Any) -> dict[str, str]:
    if value in (None, "", {}):
        return {}
    if not isinstance(value, dict):
        fail("browser_credential must be an object")
    email = str(value.get("email", "")).strip()
    password = str(value.get("password", ""))
    if not email or not password:
        fail("browser_credential requires email and password")
    return {"email": email, "password": password}


def cookie_payload(value: Any) -> list[dict[str, Any]]:
    if value in (None, "", []):
        return []
    if isinstance(value, str):
        try:
            value = json.loads(value)
        except json.JSONDecodeError as exc:
            fail(f"invalid cookies JSON: {exc}")
    if not isinstance(value, list):
        fail("cookies must be a JSON array")
    return value


def required_env(name: str, default: str = "") -> str:
    value = os.getenv(name, default).strip()
    if not value:
        fail(f"{name} is not configured")
    return value


def parse_transactions(output: str, merchant_id: str) -> list[dict[str, Any]]:
    decoder = json.JSONDecoder()
    payload = None
    for index, character in enumerate(output):
        if character != "{":
            continue
        try:
            candidate, _ = decoder.raw_decode(output[index:])
        except json.JSONDecodeError:
            continue
        if isinstance(candidate, dict) and "transactions" in candidate:
            payload = candidate
    if payload is None:
        fail("browser output does not contain a transactions JSON object")
    rows = payload.get("transactions") if isinstance(payload, dict) else None
    if not isinstance(rows, list):
        fail("browser output requires a transactions array")

    transactions = []
    for row in rows:
        if not isinstance(row, dict):
            continue
        reference = str(row.get("reference", "")).strip()
        amount = row.get("amount")
        paid_at = str(row.get("paid_at", "")).strip()
        if not reference or not isinstance(amount, int) or not paid_at:
            continue
        transactions.append(
            {
                "reference": reference,
                "amount": amount,
                "status": str(row.get("status", "paid")).strip() or "paid",
                "paid_at": paid_at,
                "merchant_id": merchant_id,
            }
        )
    return transactions


def run(payload: dict[str, Any]) -> list[dict[str, Any]]:
    merchant_id = str(payload["merchant_id"])
    cookies = cookie_payload(payload.get("cookies"))
    credential = browser_credential(payload.get("browser_credential"))
    portal_url = required_env("WEBWRIGHT_PORTAL_URL", "https://merchant.qris.interactive.co.id")
    history_path = os.getenv("WEBWRIGHT_HISTORY_PATH", "/v2/m/kontenr.php?idir=pages/historytrx.php")
    row_selector = required_env("WEBWRIGHT_TRANSACTION_ROW_SELECTOR")
    profile_root = Path(os.getenv("WEBWRIGHT_PROFILE_DIR", ".webwright-profiles"))
    profile_dir = profile_root / merchant_id
    profile_dir.mkdir(parents=True, exist_ok=True)

    manual_login = os.getenv("WEBWRIGHT_MANUAL_LOGIN", "").lower() == "true"
    if manual_login:
        script = f"""
import json
cookies = {json.dumps(cookies)}
credential = {json.dumps(credential)}
if cookies:
    await context.add_cookies(cookies)
await page.goto({json.dumps(portal_url + history_path)}, wait_until="domcontentloaded")
email_input = page.locator({json.dumps(os.getenv("WEBWRIGHT_LOGIN_EMAIL_SELECTOR", "input[type='email'], input[name*='email' i], input[placeholder*='email' i], input[type='text']"))})
password_input = page.locator({json.dumps(os.getenv("WEBWRIGHT_LOGIN_PASSWORD_SELECTOR", "input[type='password']"))})
if credential and await email_input.count() and await password_input.count():
    await email_input.first.fill(credential["email"])
    await password_input.first.fill(credential["password"])
await page.wait_for_function("() => !document.querySelector('input[type=\"password\"]')", timeout=600000)
print(json.dumps({{"transactions": []}}))
"""
    else:
        script = f"""
import json
cookies = {json.dumps(cookies)}
credential = {json.dumps(credential)}
if cookies:
    await context.add_cookies(cookies)
await page.goto({json.dumps(portal_url + history_path)}, wait_until="domcontentloaded")
email_input = page.locator({json.dumps(os.getenv("WEBWRIGHT_LOGIN_EMAIL_SELECTOR", "input[type='email'], input[name*='email' i], input[placeholder*='email' i], input[type='text']"))})
password_input = page.locator({json.dumps(os.getenv("WEBWRIGHT_LOGIN_PASSWORD_SELECTOR", "input[type='password']"))})
if credential and await email_input.count() and await password_input.count():
    await email_input.first.fill(credential["email"])
    await password_input.first.fill(credential["password"])
    await page.locator({json.dumps(os.getenv("WEBWRIGHT_LOGIN_SUBMIT_SELECTOR", "button[type='submit'], input[type='submit'], button:has-text('Masuk')"))}).first.click()
    await page.wait_for_load_state("networkidle", timeout=15000)
if await email_input.count() and await password_input.count():
    raise RuntimeError("portal login is still displayed; verify browser email and password")
await page.goto({json.dumps(portal_url + history_path)}, wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator({json.dumps(row_selector)}).evaluate_all('''
(rows) => rows.map((row) => {{
  const value = (selector) => selector ? row.querySelector(selector)?.textContent?.trim() || "" : "";
  const amountText = value({json.dumps(os.getenv("WEBWRIGHT_AMOUNT_SELECTOR", "[data-field='amount']"))});
  const digits = amountText.replace(/[^0-9-]/g, "");
  const paidAtText = value({json.dumps(os.getenv("WEBWRIGHT_PAID_AT_SELECTOR", "[data-field='paid_at']"))}).replace(/\s+/g, " ");
  const match = paidAtText.match(/(\d{{2}})\/(\d{{2}})\/(\d{{4}})\s+(\d{{2}}):(\d{{2}}):(\d{{2}})/);
  return {{
    reference: value({json.dumps(os.getenv("WEBWRIGHT_REFERENCE_SELECTOR", "[data-field='reference']"))}),
    amount: Number.parseInt(digits, 10),
    status: value({json.dumps(os.getenv("WEBWRIGHT_STATUS_SELECTOR", "[data-field='status']"))}) || "paid",
    paid_at: match ? `${{match[3]}}-${{match[2]}}-${{match[1]}}T${{match[4]}}:${{match[5]}}:${{match[6]}}+07:00` : "",
  }};
}})
''')
print(json.dumps({{"transactions": rows}}))
"""

    environment = LocalBrowserEnvironment(
        browser_mode=os.getenv("WEBWRIGHT_BROWSER_MODE", "cloak_persistent"),
        headless=False if manual_login else os.getenv("WEBWRIGHT_HEADLESS", "true").lower() == "true",
        start_url=portal_url + history_path,
        user_data_dir=profile_dir,
        output_dir=profile_dir / "outputs",
        browser_navigation_timeout_ms=30000,
        step_execution_timeout_ms=660000 if manual_login else 45000,
    )
    try:
        with contextlib.redirect_stdout(sys.stderr):
            environment.prepare(task="merchant transaction sync", start_url=portal_url + history_path)
            result = environment.execute({"python_code": script})
        if result.get("returncode") != 0:
            observation = result.get("observation", {})
            fail(observation.get("exception") or "Webwright transaction extraction failed")
        return parse_transactions(result.get("output", ""), merchant_id)
    finally:
        environment.close()


def main() -> int:
    payload = load_request()
    transactions = run(payload)
    json.dump({"transactions": transactions}, sys.stdout)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
