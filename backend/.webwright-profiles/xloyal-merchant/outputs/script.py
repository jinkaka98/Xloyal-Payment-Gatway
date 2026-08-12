

# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
if credential and page.url.includes("login"):
    await page.locator("input[type='email']").fill(credential["email"])
    await page.locator("input[type='password']").fill(credential["password"])
    await page.locator("button[type='submit']").click()
    await page.wait_for_load_state("networkidle", timeout=15000)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all("""
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
""")
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
if credential and "login" in page.url:
    await page.locator("input[type='email']").fill(credential["email"])
    await page.locator("input[type='password']").fill(credential["password"])
    await page.locator("button[type='submit']").click()
    await page.wait_for_load_state("networkidle", timeout=15000)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all("""
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
""")
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
if credential and "login" in page.url:
    await page.locator("input[type='email']").fill(credential["email"])
    await page.locator("input[type='password']").fill(credential["password"])
    await page.locator("button[type='submit']").click()
    await page.wait_for_load_state("networkidle", timeout=15000)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all("""
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
""")
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
if credential and "login" in page.url:
    await page.locator("input[type='email']").fill(credential["email"])
    await page.locator("input[type='password']").fill(credential["password"])
    await page.locator("button[type='submit']").click()
    await page.wait_for_load_state("networkidle", timeout=15000)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all("""
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
""")
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
if credential and "login" in page.url:
    await page.locator("input[type='email']").fill(credential["email"])
    await page.locator("input[type='password']").fill(credential["password"])
    await page.locator("button[type='submit']").click()
    await page.wait_for_load_state("networkidle", timeout=15000)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all("""
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
""")
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
if credential and "login" in page.url:
    await page.locator("input[type='email']").fill(credential["email"])
    await page.locator("input[type='password']").fill(credential["password"])
    await page.locator("button[type='submit']").click()
    await page.wait_for_load_state("networkidle", timeout=15000)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all("""
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
""")
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
if credential and "login" in page.url:
    await page.locator("input[type='email']").fill(credential["email"])
    await page.locator("input[type='password']").fill(credential["password"])
    await page.locator("button[type='submit']").click()
    await page.wait_for_load_state("networkidle", timeout=15000)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all("""
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
""")
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
if credential and "login" in page.url:
    await page.locator("input[type='email']").fill(credential["email"])
    await page.locator("input[type='password']").fill(credential["password"])
    await page.locator("button[type='submit']").click()
    await page.wait_for_load_state("networkidle", timeout=15000)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all("""
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
""")
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
if credential and "login" in page.url:
    await page.locator("input[type='email']").fill(credential["email"])
    await page.locator("input[type='password']").fill(credential["password"])
    await page.locator("button[type='submit']").click()
    await page.wait_for_load_state("networkidle", timeout=15000)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all("""
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
""")
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
email_input = page.locator("input[type='email']")
if credential and await email_input.count():
    await email_input.first.fill(credential["email"])
    await page.locator("input[type='password']").first.fill(credential["password"])
    await page.locator("button[type='submit']").first.click()
    await page.wait_for_load_state("networkidle", timeout=15000)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all("""
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
""")
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
email_input = page.locator("input[type='email']")
if credential and await email_input.count():
    await email_input.first.fill(credential["email"])
    await page.locator("input[type='password']").first.fill(credential["password"])
    await page.locator("button[type='submit']").first.click()
    await page.wait_for_load_state("networkidle", timeout=15000)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all("""
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
""")
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
email_input = page.locator("input[type='email'], input[name*='email' i], input[placeholder*='email' i], input[type='text']")
password_input = page.locator("input[type='password']")
if credential and await email_input.count() and await password_input.count():
    await email_input.first.fill(credential["email"])
    await password_input.first.fill(credential["password"])
    await page.locator("button[type='submit'], input[type='submit'], button:has-text('Masuk')").first.click()
    await page.wait_for_load_state("networkidle", timeout=15000)
if await email_input.count() and await password_input.count():
    raise RuntimeError("portal login is still displayed; verify browser email and password")
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all("""
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
""")
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
email_input = page.locator("input[type='email'], input[name*='email' i], input[placeholder*='email' i], input[type='text']")
password_input = page.locator("input[type='password']")
if credential and await email_input.count() and await password_input.count():
    await email_input.first.fill(credential["email"])
    await password_input.first.fill(credential["password"])
    await page.locator("button[type='submit'], input[type='submit'], button:has-text('Masuk')").first.click()
    await page.wait_for_load_state("networkidle", timeout=15000)
if await email_input.count() and await password_input.count():
    raise RuntimeError("portal login is still displayed; verify browser email and password")
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all("""
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
""")
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
email_input = page.locator("input[type='email'], input[name*='email' i], input[placeholder*='email' i], input[type='text']")
password_input = page.locator("input[type='password']")
if credential and await email_input.count() and await password_input.count():
    await email_input.first.fill(credential["email"])
    await password_input.first.fill(credential["password"])
    await page.locator("button[type='submit'], input[type='submit'], button:has-text('Masuk')").first.click()
    await page.wait_for_load_state("networkidle", timeout=15000)
if await email_input.count() and await password_input.count():
    raise RuntimeError("portal login is still displayed; verify browser email and password")
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all("""
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
""")
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
email_input = page.locator("input[type='email'], input[name*='email' i], input[placeholder*='email' i], input[type='text']")
password_input = page.locator("input[type='password']")
if credential and await email_input.count() and await password_input.count():
    await email_input.first.fill(credential["email"])
    await password_input.first.fill(credential["password"])
    await page.locator("button[type='submit'], input[type='submit'], button:has-text('Masuk')").first.click()
    await page.wait_for_load_state("networkidle", timeout=15000)
if await email_input.count() and await password_input.count():
    raise RuntimeError("portal login is still displayed; verify browser email and password")
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all("""
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
""")
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
email_input = page.locator("input[type='email'], input[name*='email' i], input[placeholder*='email' i], input[type='text']")
password_input = page.locator("input[type='password']")
if credential and await email_input.count() and await password_input.count():
    await email_input.first.fill(credential["email"])
    await password_input.first.fill(credential["password"])
    await page.locator("button[type='submit'], input[type='submit'], button:has-text('Masuk')").first.click()
    await page.wait_for_load_state("networkidle", timeout=15000)
if await email_input.count() and await password_input.count():
    raise RuntimeError("portal login is still displayed; verify browser email and password")
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all('''
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
''')
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
email_input = page.locator("input[type='email'], input[name*='email' i], input[placeholder*='email' i], input[type='text']")
password_input = page.locator("input[type='password']")
if credential and await email_input.count() and await password_input.count():
    await email_input.first.fill(credential["email"])
    await password_input.first.fill(credential["password"])
    await page.locator("button[type='submit'], input[type='submit'], button:has-text('Masuk')").first.click()
    await page.wait_for_load_state("networkidle", timeout=15000)
if await email_input.count() and await password_input.count():
    raise RuntimeError("portal login is still displayed; verify browser email and password")
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("[data-transaction-row]").evaluate_all('''
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("[data-field='amount']");
  const digits = amountText.replace(/[^0-9-]/g, "");
  return {
    reference: value("[data-field='reference']"),
    amount: Number.parseInt(digits, 10),
    status: value("[data-field='status']") || "paid",
    paid_at: value("[data-field='paid_at']"),
  };
})
''')
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
email_input = page.locator("input[type='email'], input[name*='email' i], input[placeholder*='email' i], input[type='text']")
password_input = page.locator("input[type='password']")
if credential and await email_input.count() and await password_input.count():
    await email_input.first.fill(credential["email"])
    await password_input.first.fill(credential["password"])
    await page.locator("button[type='submit'], input[type='submit'], button:has-text('Masuk')").first.click()
    await page.wait_for_load_state("networkidle", timeout=15000)
if await email_input.count() and await password_input.count():
    raise RuntimeError("portal login is still displayed; verify browser email and password")
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("table tbody tr").evaluate_all('''
(rows) => rows.map((row) => {
  const value = (selector) => row.querySelector(selector)?.textContent?.trim() || "";
  const amountText = value("td:nth-child(3)");
  const digits = amountText.replace(/[^0-9-]/g, "");
  const paidAtText = value("td:nth-child(2)").replace(/\s+/g, " ");
  const match = paidAtText.match(/(\d{2})\/(\d{2})\/(\d{4})\s+(\d{2}):(\d{2}):(\d{2})/);
  return {
    reference: value("td:nth-child(9)"),
    amount: Number.parseInt(digits, 10),
    status: value("td:nth-child(4)") || "paid",
    paid_at: match ? `${match[3]}-${match[2]}-${match[1]}T${match[4]}:${match[5]}:${match[6]}+07:00` : "",
  };
})
''')
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
email_input = page.locator("input[type='email'], input[name*='email' i], input[placeholder*='email' i], input[type='text']")
password_input = page.locator("input[type='password']")
if credential and await email_input.count() and await password_input.count():
    await email_input.first.fill(credential["email"])
    await password_input.first.fill(credential["password"])
    await page.locator("button[type='submit'], input[type='submit'], button:has-text('Masuk')").first.click()
    await page.wait_for_load_state("networkidle", timeout=15000)
if await email_input.count() and await password_input.count():
    raise RuntimeError("portal login is still displayed; verify browser email and password")
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("table tbody tr").evaluate_all('''
(rows) => rows.map((row) => {
  const value = (selector) => selector ? row.querySelector(selector)?.textContent?.trim() || "" : "";
  const amountText = value("td:nth-child(3)");
  const digits = amountText.replace(/[^0-9-]/g, "");
  const paidAtText = value("td:nth-child(2)").replace(/\s+/g, " ");
  const match = paidAtText.match(/(\d{2})\/(\d{2})\/(\d{4})\s+(\d{2}):(\d{2}):(\d{2})/);
  return {
    reference: value("td:nth-child(9)"),
    amount: Number.parseInt(digits, 10),
    status: value("") || "paid",
    paid_at: match ? `${match[3]}-${match[2]}-${match[1]}T${match[4]}:${match[5]}:${match[6]}+07:00` : "",
  };
})
''')
print(json.dumps({"transactions": rows}))



# Step 1

import json
cookies = []
credential = {"email": "nurulnisya217@gmail.com", "password": "@Merdeka321"}
if cookies:
    await context.add_cookies(cookies)
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
email_input = page.locator("input[type='email'], input[name*='email' i], input[placeholder*='email' i], input[type='text']")
password_input = page.locator("input[type='password']")
if credential and await email_input.count() and await password_input.count():
    await email_input.first.fill(credential["email"])
    await password_input.first.fill(credential["password"])
    await page.locator("button[type='submit'], input[type='submit'], button:has-text('Masuk')").first.click()
    await page.wait_for_load_state("networkidle", timeout=15000)
if await email_input.count() and await password_input.count():
    raise RuntimeError("portal login is still displayed; verify browser email and password")
await page.goto("https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php", wait_until="domcontentloaded")
await page.wait_for_load_state("networkidle", timeout=15000)
rows = await page.locator("table tbody tr").evaluate_all('''
(rows) => rows.map((row) => {
  const value = (selector) => selector ? row.querySelector(selector)?.textContent?.trim() || "" : "";
  const amountText = value("td:nth-child(3)");
  const digits = amountText.replace(/[^0-9-]/g, "");
  const paidAtText = value("td:nth-child(2)").replace(/\s+/g, " ");
  const match = paidAtText.match(/(\d{2})\/(\d{2})\/(\d{4})\s+(\d{2}):(\d{2}):(\d{2})/);
  return {
    reference: value("td:nth-child(9)"),
    amount: Number.parseInt(digits, 10),
    status: value("") || "paid",
    paid_at: match ? `${match[3]}-${match[2]}-${match[1]}T${match[4]}:${match[5]}:${match[6]}+07:00` : "",
  };
})
''')
print(json.dumps({"transactions": rows}))

