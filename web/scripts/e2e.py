"""浏览器端到端验收短链接控制台的主流程。"""

from __future__ import annotations

import atexit
import hashlib
import hmac
import json
import os
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

from playwright.sync_api import sync_playwright


BASE_URL = os.getenv("SHORTURL_WEB_URL", "http://127.0.0.1:5173")
API_URL = os.getenv("SHORTURL_API_URL", "http://127.0.0.1:18080")
DEFAULT_WINDOWS_CHROME = Path(r"C:\Program Files\Google\Chrome\Application\chrome.exe")
CHROME_PATH = os.getenv("PLAYWRIGHT_CHROME_PATH")
if not CHROME_PATH and DEFAULT_WINDOWS_CHROME.exists():
    CHROME_PATH = str(DEFAULT_WINDOWS_CHROME)
ARTIFACT_DIR = Path(
    os.getenv(
        "SHORTURL_E2E_ARTIFACT_DIR",
        str(Path(__file__).resolve().parent.parent / ".tmp" / "playwright"),
    )
)
ARTIFACT_DIR.mkdir(parents=True, exist_ok=True)
WEBHOOK_TARGET_HOST = os.getenv(
    "SHORTURL_WEBHOOK_TARGET_HOST",
    "host.docker.internal",
)


class WebhookReceiver:
    """在本机接收并校验 API Worker 发出的 Webhook。"""

    def __init__(self) -> None:
        self.secret = ""
        self.received = threading.Event()
        self.signature_valid = False
        self.event_type = ""
        self.event_id = ""
        self.payload: dict[str, object] = {}
        self.closed = False
        receiver = self

        class Handler(BaseHTTPRequestHandler):
            def do_POST(self) -> None:  # noqa: N802
                body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
                timestamp = self.headers.get("X-ShortURL-Timestamp", "")
                expected = "sha256=" + hmac.new(
                    receiver.secret.encode(),
                    timestamp.encode() + b"." + body,
                    hashlib.sha256,
                ).hexdigest()
                receiver.signature_valid = hmac.compare_digest(
                    expected,
                    self.headers.get("X-ShortURL-Signature", ""),
                )
                receiver.event_type = self.headers.get("X-ShortURL-Event-Type", "")
                receiver.event_id = self.headers.get("X-ShortURL-Event-ID", "")
                receiver.payload = json.loads(body)
                self.send_response(204)
                self.end_headers()
                receiver.received.set()

            def log_message(self, _format: str, *_args: object) -> None:
                return

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        atexit.register(self.close)

    @property
    def endpoint_url(self) -> str:
        port = self.server.server_address[1]
        return f"http://{WEBHOOK_TARGET_HOST}:{port}/shorturl"

    def close(self) -> None:
        if self.closed:
            return
        self.closed = True
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=2)


def main() -> None:
    suffix = uuid.uuid4().hex[:8]
    email = f"web-{suffix}@example.com"
    test_ip = f"198.51.100.{1 + int(suffix[:2], 16) % 250}"
    alias = f"web{suffix}"
    trial_alias = f"trial{suffix}"
    hostname = f"go-{suffix}.example.com"
    console_errors: list[str] = []
    webhook_receiver = WebhookReceiver()

    with sync_playwright() as playwright:
        launch_options = {"headless": True}
        if CHROME_PATH:
            launch_options["executable_path"] = CHROME_PATH
        browser = playwright.chromium.launch(**launch_options)
        context = browser.new_context(
            viewport={"width": 1440, "height": 980},
            extra_http_headers={"X-Forwarded-For": test_ip},
        )
        page = context.new_page()
        page.on(
            "console",
            lambda message: console_errors.append(message.text)
            if message.type == "error"
            else None,
        )

        page.goto(BASE_URL)
        page.wait_for_url("**/try")
        page.wait_for_load_state("networkidle")
        page.get_by_label("目标地址").fill("https://example.com/trial")
        page.get_by_label("自定义短码，可选").fill(trial_alias)
        page.get_by_label("标题，可选").fill("免登录试用")
        page.get_by_role("button", name="生成试用短链").click()
        page.get_by_role("heading", name="免登录试用").wait_for()
        trial_short_url = page.locator(".trial-result a").inner_text()
        trial_response = context.request.get(trial_short_url, max_redirects=0)
        assert trial_response.status == 302, trial_response.text()
        page.screenshot(path=ARTIFACT_DIR / "trial.png", full_page=True)

        page.goto(f"{BASE_URL}/register")
        page.wait_for_load_state("networkidle")
        page.screenshot(path=ARTIFACT_DIR / "register.png", full_page=True)

        page.get_by_label("显示名称").fill("浏览器验收")
        page.get_by_label("邮箱").fill(email)
        page.get_by_label("密码", exact=True).fill("password123")
        page.get_by_label("确认密码").fill("password123")
        page.get_by_role("button", name="创建账号").click()
        page.wait_for_url("**/dashboard")
        page.wait_for_load_state("networkidle")
        page.get_by_text("已认领 1 条试用短链，点击数据也已保留。").wait_for()
        page.locator(".recent-list").get_by_text("免登录试用", exact=True).wait_for()
        page.get_by_text("整体点击趋势").wait_for()
        total_links = page.locator(".metric-cell").filter(
            has_text="全部链接",
        ).locator("strong").inner_text()
        assert total_links == "1", f"expected 1 workspace link, got {total_links}"

        page.screenshot(path=ARTIFACT_DIR / "dashboard.png", full_page=True)

        page.get_by_role("link", name="开发者").click()
        page.wait_for_url("**/developer")
        openapi_response = context.request.get(f"{BASE_URL}/openapi.yaml")
        assert openapi_response.status == 200, openapi_response.text()
        assert "openapi: 3.1.0" in openapi_response.text()
        page.locator(".developer-grid").get_by_label("名称", exact=True).fill(
            "浏览器验收 Key",
        )
        page.get_by_role("button", name="创建 API Key").click()
        api_key = page.locator(".api-secret-value").inner_text()
        assert api_key.startswith("sl_live_"), api_key
        api_key_response = context.request.get(
            f"{API_URL}/api/v1/analytics/overview?days=7",
            headers={"Authorization": f"Bearer {api_key}"},
        )
        assert api_key_response.status == 200, api_key_response.text()
        page.once("dialog", lambda dialog: dialog.accept())
        page.get_by_role("button", name="撤销", exact=True).click()
        page.locator(".api-key-row .status-pill").get_by_text("已撤销").wait_for()
        revoked_response = context.request.get(
            f"{API_URL}/api/v1/analytics/overview?days=7",
            headers={"Authorization": f"Bearer {api_key}"},
        )
        assert revoked_response.status == 401, revoked_response.text()

        webhook_panel = page.locator(".webhook-feature")
        webhook_panel.get_by_label("名称").fill("浏览器验收 Webhook")
        webhook_panel.get_by_label("回调地址").fill(webhook_receiver.endpoint_url)
        webhook_panel.get_by_role("button", name="创建 Webhook").click()
        webhook_secret = webhook_panel.locator(".webhook-secret-value").inner_text()
        assert webhook_secret.startswith("whsec_"), webhook_secret
        webhook_receiver.secret = webhook_secret
        page.screenshot(path=ARTIFACT_DIR / "developer.png", full_page=True)

        page.get_by_role("link", name="域名").click()
        page.wait_for_url("**/domains")
        page.get_by_role("textbox", name="域名", exact=True).fill(hostname)
        page.get_by_role("button", name="添加域名").click()
        page.get_by_role("heading", name=hostname, exact=True).wait_for()
        page.screenshot(path=ARTIFACT_DIR / "domains.png", full_page=True)
        page.get_by_role("button", name="验证域名").click()
        page.get_by_text("已验证").wait_for()

        page.get_by_role("link", name="工作台").click()
        page.wait_for_url("**/dashboard")
        page.get_by_role("button", name="创建短链接").click()
        dialog = page.get_by_role("dialog")
        dialog.get_by_label("域名").select_option(label=hostname)
        dialog.get_by_label("目标地址").fill("https://example.com/original")
        dialog.get_by_label("自定义短码").fill(alias)
        dialog.get_by_label("标题").fill("浏览器验收短链")
        dialog.get_by_role("button", name="创建短链").click()
        page.locator(".recent-list").get_by_text("浏览器验收短链").wait_for()

        assert webhook_receiver.received.wait(timeout=15), "Webhook 未在 15 秒内送达"
        assert webhook_receiver.signature_valid, "Webhook HMAC 校验失败"
        assert webhook_receiver.event_type == "link.created", webhook_receiver.event_type
        assert webhook_receiver.event_id, "Webhook 缺少 Event ID"
        assert webhook_receiver.payload.get("type") == "link.created", (
            webhook_receiver.payload
        )

        page.get_by_role("link", name="开发者").click()
        page.wait_for_url("**/developer")
        webhook_panel.get_by_role("button", name="查看投递").click()
        webhook_panel.get_by_text("已送达").wait_for()
        page.screenshot(path=ARTIFACT_DIR / "webhook-delivery.png", full_page=True)
        page.get_by_role("link", name="工作台").click()
        page.wait_for_url("**/dashboard")

        page.locator(".recent-list").get_by_role(
            "link", name="浏览器验收短链",
        ).click()
        page.wait_for_url("**/links/**")
        page.wait_for_load_state("networkidle")
        page.get_by_role("heading", name="浏览器验收短链").wait_for()

        request = context.request
        for user_agent in (
            "Mozilla/5.0 (Windows NT 10.0) Chrome/140.0.0.0 Safari/537.36",
            "Mozilla/5.0 (Windows NT 10.0) Chrome/140.0.0.0 Safari/537.36",
            "Googlebot/2.1",
        ):
            response = request.get(
                f"{API_URL}/{alias}",
                headers={"User-Agent": user_agent, "Host": hostname},
                max_redirects=0,
            )
            assert response.status == 302, response.text()

        time.sleep(2)
        page.reload()
        page.wait_for_load_state("networkidle")
        page.locator(".summary-primary strong").wait_for()
        total_clicks = page.locator(".summary-primary strong").inner_text()
        assert total_clicks == "3", f"expected 3 clicks, got {total_clicks}"
        page.screenshot(path=ARTIFACT_DIR / "detail-desktop.png", full_page=True)

        with page.expect_download() as png_download:
            page.get_by_role("button", name="下载 PNG").click()
        png_download.value.save_as(ARTIFACT_DIR / f"{alias}.png")

        with page.expect_download() as svg_download:
            page.get_by_role("button", name="下载 SVG").click()
        svg_download.value.save_as(ARTIFACT_DIR / f"{alias}.svg")

        page.get_by_role("button", name="编辑链接").click()
        edit_dialog = page.get_by_role("dialog")
        edit_dialog.get_by_label("目标地址").fill("https://example.com/changed")
        edit_dialog.get_by_role("button", name="保存修改").click()
        page.get_by_text("https://example.com/changed").wait_for()

        mobile = context.new_page()
        mobile.set_viewport_size({"width": 390, "height": 844})
        mobile.goto(f"{BASE_URL}/links")
        mobile.wait_for_load_state("networkidle")
        mobile.screenshot(path=ARTIFACT_DIR / "links-mobile.png", full_page=True)
        mobile.goto(f"{BASE_URL}/developer")
        mobile.wait_for_load_state("networkidle")
        mobile.locator(".webhook-feature").wait_for()
        mobile.screenshot(
            path=ARTIFACT_DIR / "developer-mobile.png",
            full_page=True,
        )
        mobile.set_viewport_size({"width": 320, "height": 844})
        mobile.reload()
        mobile.wait_for_load_state("networkidle")
        assert mobile.evaluate(
            "document.documentElement.scrollWidth <= window.innerWidth",
        ), "320px 页面出现横向溢出"
        mobile.screenshot(
            path=ARTIFACT_DIR / "developer-mobile-320.png",
            full_page=True,
        )

        if console_errors:
            raise AssertionError(f"browser console errors: {console_errors}")

        print(
            {
                "email": email,
                "alias": alias,
                "hostname": hostname,
                "total_clicks": total_clicks,
                "screenshots": str(ARTIFACT_DIR),
            }
        )
        browser.close()
        webhook_receiver.close()


if __name__ == "__main__":
    main()
