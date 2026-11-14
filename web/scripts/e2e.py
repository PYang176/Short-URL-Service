"""浏览器端到端验收普通用户的短链、二维码和访问分析流程。"""

from __future__ import annotations

import csv
import os
import time
import uuid
from datetime import datetime
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


def main() -> None:
    suffix = uuid.uuid4().hex[:8]
    email = f"web-{suffix}@example.com"
    test_ip = f"198.51.100.{1 + int(suffix[:2], 16) % 250}"
    trial_alias = f"trial{suffix}"
    alias = f"web{suffix}"
    console_errors: list[str] = []

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
        page.get_by_label("链接有效期").select_option("15")
        page.get_by_role("button", name="生成试用短链").click()
        page.get_by_role("heading", name="免登录试用").wait_for()
        trial_short_url = page.locator(".trial-result a").inner_text()
        trial_response = context.request.get(trial_short_url, max_redirects=0)
        assert trial_response.status == 302, trial_response.text()
        trial_links = context.request.get(f"{API_URL}/api/v1/trial/links").json()
        trial_link = trial_links["items"][0]
        created_at = datetime.fromisoformat(trial_link["created_at"].replace("Z", "+00:00"))
        expires_at = datetime.fromisoformat(trial_link["expires_at"].replace("Z", "+00:00"))
        lifetime = expires_at - created_at
        assert 14 * 60 <= lifetime.total_seconds() <= 15 * 60 + 5, lifetime
        page.screenshot(path=ARTIFACT_DIR / "trial.png", full_page=True)

        page.goto(f"{BASE_URL}/register")
        page.wait_for_load_state("networkidle")
        page.get_by_label("显示名称").fill("浏览器验收")
        page.get_by_label("邮箱").fill(email)
        page.get_by_label("密码", exact=True).fill("password123")
        page.get_by_label("确认密码").fill("password123")
        page.get_by_role("button", name="创建账号").click()
        page.wait_for_url("**/dashboard")
        page.wait_for_load_state("networkidle")
        page.get_by_text("已认领 1 条试用短链，点击数据也已保留。").wait_for()

        nav_labels = page.locator(".primary-nav a").all_inner_texts()
        assert [label.strip() for label in nav_labels] == ["工作台", "短链接"], nav_labels
        assert page.get_by_role("link", name="开发者").count() == 0
        assert page.get_by_role("link", name="域名").count() == 0

        page.get_by_role("button", name="创建短链接").click()
        dialog = page.get_by_role("dialog")
        dialog.get_by_label("目标地址").fill("https://example.com/original")
        dialog.get_by_label("自定义短码").fill(alias)
        dialog.get_by_label("标题").fill("浏览器验收短链")
        dialog.get_by_role("button", name="创建短链").click()
        page.locator(".recent-list").get_by_text("浏览器验收短链").wait_for()
        page.get_by_text("整体点击趋势").wait_for()
        page.screenshot(path=ARTIFACT_DIR / "dashboard-desktop.png", full_page=True)

        access_token = page.evaluate(
            "localStorage.getItem('shorturl_access_token')",
        )
        assert access_token, "缺少普通用户 Access Token"
        admin_response = context.request.get(
            f"{API_URL}/api/v1/api-keys",
            headers={"Authorization": f"Bearer {access_token}"},
        )
        assert admin_response.status == 403, admin_response.text()
        metrics_response = context.request.get(f"{API_URL}/metrics")
        assert metrics_response.status == 404, metrics_response.text()

        page.goto(f"{BASE_URL}/developer")
        page.wait_for_url("**/dashboard")
        page.goto(f"{BASE_URL}/domains")
        page.wait_for_url("**/dashboard")

        page.goto(f"{BASE_URL}/report")
        page.wait_for_load_state("networkidle")
        page.get_by_label("可疑短链").fill(f"{BASE_URL}/r/{alias}")
        page.get_by_label("风险类型").select_option("phishing")
        page.get_by_label("补充说明，可选").fill("浏览器验收举报")
        page.get_by_role("button", name="提交举报").click()
        page.get_by_role("heading", name="举报已提交").wait_for()
        assert page.locator("code").inner_text(), "举报编号为空"
        page.goto(f"{BASE_URL}/dashboard")
        page.wait_for_load_state("networkidle")

        page.locator(".recent-list").get_by_role(
            "link",
            name="浏览器验收短链",
        ).click()
        page.wait_for_url("**/links/**")
        page.wait_for_load_state("networkidle")
        page.get_by_role("heading", name="浏览器验收短链").wait_for()

        for user_agent in (
            "Mozilla/5.0 (Windows NT 10.0) Chrome/140.0.0.0 Safari/537.36",
            "Mozilla/5.0 (Windows NT 10.0) Chrome/140.0.0.0 Safari/537.36",
            "Googlebot/2.1",
        ):
            response = context.request.get(
                f"{API_URL}/r/{alias}",
                headers={"User-Agent": user_agent},
                max_redirects=0,
            )
            assert response.status == 302, response.text()

        time.sleep(2)
        page.reload()
        page.wait_for_load_state("networkidle")
        page.locator(".summary-primary strong").wait_for()
        total_clicks = page.locator(".summary-primary strong").inner_text()
        assert total_clicks == "3", f"expected 3 clicks, got {total_clicks}"
        page.screenshot(path=ARTIFACT_DIR / "link-detail.png", full_page=True)

        export_panel = page.locator(".analytics-export-panel")
        export_panel.get_by_label("访问类型").select_option("human")
        with page.expect_download() as csv_download:
            export_panel.get_by_role("button", name="导出 CSV").click()
        export_path = ARTIFACT_DIR / f"{alias}-clicks.csv"
        csv_download.value.save_as(export_path)
        with export_path.open(encoding="utf-8-sig", newline="") as exported_file:
            exported_rows = list(csv.reader(exported_file))
        assert exported_rows[0][0] == "event_id", exported_rows[0]
        assert len(exported_rows) == 3, f"expected 2 human rows, got {len(exported_rows) - 1}"
        assert "visitor_hash" not in exported_rows[0]

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
        mobile.goto(f"{BASE_URL}/dashboard")
        mobile.wait_for_load_state("networkidle")
        mobile.screenshot(
            path=ARTIFACT_DIR / "dashboard-mobile-390.png",
            full_page=True,
        )
        mobile.set_viewport_size({"width": 320, "height": 844})
        mobile.reload()
        mobile.wait_for_load_state("networkidle")
        assert mobile.evaluate(
            "document.documentElement.scrollWidth <= window.innerWidth",
        ), "320px 页面出现横向溢出"
        mobile.screenshot(
            path=ARTIFACT_DIR / "dashboard-mobile-320.png",
            full_page=True,
        )

        unexpected_console_errors = [
            message
            for message in console_errors
            if "status of 403" not in message and "status of 404" not in message
        ]
        if unexpected_console_errors:
            raise AssertionError(
                f"browser console errors: {unexpected_console_errors}",
            )

        print(
            {
                "email": email,
                "alias": alias,
                "total_clicks": total_clicks,
                "screenshots": str(ARTIFACT_DIR),
            }
        )
        browser.close()


if __name__ == "__main__":
    main()
