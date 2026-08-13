import json
import os
import re
import subprocess
import threading
import time
from datetime import datetime
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


DISPLAY = os.environ.get("DISPLAY", ":99.0")
PORTAL_LOGIN = "https://merchant.qris.interactive.co.id/v2/m/login/"
PORTAL_HISTORY = "https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php"
LOCK = threading.Lock()


def xdotool(*args):
    return subprocess.run(
        ["xdotool", *map(str, args)],
        env={**os.environ, "DISPLAY": DISPLAY},
        text=True,
        capture_output=True,
        check=True,
    ).stdout.strip()


def clipboard():
    last = "clipboard is empty"
    for _ in range(10):
        result = subprocess.run(
            ["xclip", "-selection", "clipboard", "-o"],
            env={**os.environ, "DISPLAY": DISPLAY},
            text=True,
            capture_output=True,
        )
        if result.returncode == 0 and result.stdout:
            return result.stdout
        last = result.stderr.strip() or last
        time.sleep(0.2)
    raise RuntimeError(last)


def chromium_window():
    windows = xdotool("search", "--class", "chromium").splitlines()
    fallback = None
    for window in windows:
        title = xdotool("getwindowname", window)
        if title not in ("chromium", ""):
            fallback = window
    if fallback:
        return fallback
    raise RuntimeError("Chromium window was not found")


def navigate(window, url):
    xdotool("windowfocus", window)
    xdotool("key", "--window", window, "ctrl+l")
    xdotool("type", "--window", window, "--delay", "2", "--", url)
    xdotool("key", "--window", window, "Return")


def current_url(window):
    xdotool("windowfocus", window)
    xdotool("key", "--window", window, "ctrl+l")
    xdotool("key", "--window", window, "ctrl+c")
    time.sleep(0.1)
    value = clipboard().strip()
    xdotool("key", "--window", window, "Escape")
    return value


def click(window, x, y):
    xdotool("mousemove", "--window", window, x, y)
    xdotool("click", "--window", window, "1")


def fill_login(window, email, password):
    navigate(window, PORTAL_LOGIN)
    time.sleep(4)
    click(window, 775, 269)
    xdotool("key", "--window", window, "ctrl+a")
    xdotool("type", "--window", window, "--delay", "15", "--", email)
    click(window, 775, 328)
    xdotool("key", "--window", window, "ctrl+a")
    xdotool("type", "--window", window, "--delay", "15", "--", password)
    click(window, 775, 430)


def wait_authenticated(window, timeout=600):
    deadline = time.time() + timeout
    while time.time() < deadline:
        title = xdotool("getwindowname", window)
        if "Login InterActive" not in title:
            return
        time.sleep(2)
    raise TimeoutError("manual login did not complete before timeout")


def scrape_history(window):
    navigate(window, PORTAL_HISTORY)
    time.sleep(6)
    click(window, 105, 350)
    time.sleep(0.5)
    now = datetime.now()
    # The left calendar is the current month. Select day 1, then today's day.
    first_x, first_y = 267, 447
    start_weekday = datetime(now.year, now.month, 1).weekday()  # Monday=0
    first_sunday_index = (start_weekday + 1) % 7
    day_index = first_sunday_index + now.day - 1
    end_x = 75 + (day_index % 7) * 32
    end_y = 447 + (day_index // 7) * 27
    click(window, first_x, first_y)
    click(window, end_x, end_y)
    click(window, 498, 629)
    click(window, 663, 350)
    xdotool("key", "--window", window, "ctrl+a")
    xdotool("type", "--window", window, "300")
    click(window, 846, 350)
    time.sleep(7)
    click(window, 122, 516)
    xdotool("type", "--window", window, "All")
    xdotool("key", "--window", window, "Return")
    time.sleep(1)
    click(window, 500, 600)
    xdotool("key", "--window", window, "ctrl+a")
    time.sleep(0.2)
    xdotool("key", "--window", window, "ctrl+c")
    time.sleep(0.8)
    copied = clipboard()
    transactions = parse_transactions(copied)
    if not transactions:
        raise RuntimeError(f"history table copy returned no rows (title={xdotool('getwindowname', window)!r}, bytes={len(copied)})")
    return transactions


def parse_transactions(text):
    transactions = []
    for line in text.splitlines():
        cells = line.split("\t")
        if len(cells) != 11 or not cells[0].isdigit():
            continue
        _, paid_at, amount, status, _, _, rrn, _, transaction_id, invoice_id, _ = cells
        match = re.fullmatch(r"(\d{2})/(\d{2})/(\d{4}) (\d{2}:\d{2}:\d{2})", paid_at.strip())
        if not match:
            continue
        transactions.append({
            "reference": " ".join(value for value in (rrn, transaction_id, invoice_id) if value.strip()),
            "amount": int(re.sub(r"\D", "", amount)),
            "status": "paid" if "sukses" in status.lower() else status.strip().lower(),
            "paid_at": f"{match[3]}-{match[2]}-{match[1]}T{match[4]}+07:00",
        })
    return transactions


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/health":
            self.send_error(404)
            return
        self.respond(200, {"status": "ok"})

    def do_POST(self):
        if self.path != "/sync":
            self.send_error(404)
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
            payload = json.loads(self.rfile.read(length) or b"{}")
            with LOCK:
                window = chromium_window()
                navigate(window, PORTAL_HISTORY)
                time.sleep(5)
                title = xdotool("getwindowname", window)
                if "Login InterActive" in title:
                    credential = payload.get("browser_credential") or {}
                    if not credential.get("email") or not credential.get("password"):
                        raise RuntimeError("portal credentials are missing")
                    fill_login(window, credential["email"], credential["password"])
                    wait_authenticated(window)
                transactions = scrape_history(window)
            self.respond(200, {"transactions": transactions})
        except TimeoutError as error:
            self.respond(409, {"error": str(error), "status": "reconnect_required"})
        except Exception as error:
            self.respond(500, {"error": str(error)})

    def respond(self, status, payload):
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_):
        return


ThreadingHTTPServer(("0.0.0.0", 9224), Handler).serve_forever()
