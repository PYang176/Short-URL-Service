"""浏览器端到端验收短链接控制台的主流程。"""

from __future__ import annotations

import os
import time
import uuid
from pathlib import Path

from playwright.sync_api import sync_playwright


BASE_URL = os.getenv("SHORTURL_WEB_URL", "http://127.0.0.1:5173")
API_URL = os.getenv("SHORTURL_API_URL", "http://127.0.0.1:18080")
DEFAULT_WINDOWS_CHROME = Path(r"C:\Program Files\Google\Chrome\Application\chrome.exe")
CHROME_PATH = os.getenv("PLAYWRIGHT_CHROME_PATH")
if not CHROME_PATH and DEFAULT_WINDOWS_CHROME.exists():
    CHROME_PATH = str(DEFAULT_WINDOWS_CHROME)
ARTIFACT_DIR = Path(__file__).resolve().parent.parent / ".tmp" / "playwright"
ARTIFACT_DIR.mkdir(parents=True, exist_ok=True)


def main() -> None:
    suffix = uuid.uuid4().hex[:8]
    email = f"web-{suffix}@example.com"
    alias = f"web{suffix}"
    trial_alias = f"trial{suffix}"
    hostname = f"go-{suffix}.example.com"
    console_errors: list[str] = []

    with sync_playwright() as playwright:
        launch_options = {"headless": True}
        if CHROME_PATH:
            launch_options["executable_path"] = CHROME_PATH
        browser = playwright.chromium.launch(**launch_options)
        context = browser.new_context(viewport={"width": 1440, "height": 980})
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


if __name__ == "__main__":
    main()
