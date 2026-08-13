
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
start_date = "2026-08-14"
end_date = "2026-08-14"
start_display = "/".join(reversed(start_date.split("-")))
end_display = "/".join(reversed(end_date.split("-")))
if start_date and end_date:
    inputs = page.locator("input")
    for index in range(await inputs.count()):
        candidate = inputs.nth(index)
        placeholder = (await candidate.get_attribute("placeholder") or "").lower()
        value = await candidate.input_value()
        if "tanggal" in placeholder or "tanggal" in value.lower() or " - " in value:
            await candidate.click()
            await candidate.press("Control+A")
            await candidate.fill(start_display + " - " + end_display)
            await candidate.press("Enter")
            await page.wait_for_timeout(500)
            break
    numeric_inputs = page.locator("input[type='number'], input[inputmode='numeric']")
    for index in range(await numeric_inputs.count()):
        candidate = numeric_inputs.nth(index)
        value = await candidate.input_value()
        if value in ("10", "", "5"):
            await candidate.fill("300")
            break
    submit = page.get_by_role("button", name="Tampilkan Hasil")
    if await submit.count():
        await submit.first.click()
        await page.wait_for_timeout(3000)
        body_text = await page.locator("body").inner_text()
        expected_range = start_display + " - " + end_display
        if expected_range not in body_text:
            raise RuntimeError("history filter was not applied; expected " + expected_range)
transactions = []
seen_references = set()
for _ in range(100):
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
    for row in rows:
        if row["reference"] and row["amount"] > 0 and row["paid_at"] and row["reference"] not in seen_references:
            transactions.append(row)
            seen_references.add(row["reference"])
    next_page = page.locator("a[rel='next'], button[aria-label='Next'], .paginate_button.next, .pagination .next a")
    if not await next_page.count():
        break
    next_button = next_page.first
    is_disabled = await next_button.evaluate("(element) => element.hasAttribute('disabled') || element.getAttribute('aria-disabled') === 'true' || element.classList.contains('disabled') || element.parentElement?.classList.contains('disabled')")
    if is_disabled:
        break
    previous = rows[0]["reference"] if rows else ""
    await next_button.click()
    await page.wait_for_timeout(750)
    current_rows = await page.locator("table tbody tr").count()
    if current_rows == 0:
        break
    current_reference = await page.locator("table tbody tr").first.locator("td:nth-child(9)").text_content()
    if previous and (current_reference or "").strip() == previous:
        break
print(json.dumps({"transactions": transactions}))
