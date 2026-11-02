"""Apply agent: after the user approves a draft, open the job's application form in a visible
browser and pre-fill it. It never presses Submit; the user reviews the form and sends it
themselves (also covers captchas, résumé upload and any questions it cannot answer)."""

from __future__ import annotations

import logging
import re
import threading
from dataclasses import dataclass, field
from typing import Callable

from .scraper import PROFILE_ARGS, USER_AGENT, ScrapeError, assert_public_url, profile_path

MAX_OPEN_SECONDS = 30 * 60

# (label regex, key) tried in order; the first match per field wins.
FIELD_RULES: list[tuple[str, str]] = [
    (r"first\s*name|given\s*name", "first_name"),
    (r"last\s*name|family\s*name|surname", "last_name"),
    (r"^\s*(full\s*)?name\b", "name"),
    (r"e-?mail", "email"),
    (r"phone|mobile", "phone"),
    (r"current\s*location|^\s*location|city", "location"),
    (r"cover\s*letter|additional\s*information|message to", "cover_note"),
]


@dataclass
class Applicant:
    name: str = ""
    email: str = ""
    phone: str = ""
    location: str = ""
    cover_note: str = ""

    def value(self, key: str) -> str:
        parts = self.name.split()
        return {
            "first_name": parts[0] if parts else "",
            "last_name": " ".join(parts[1:]),
            "name": self.name,
            "email": self.email,
            "phone": self.phone,
            "location": self.location,
            "cover_note": self.cover_note,
        }.get(key, "")


@dataclass
class ApplyResult:
    filled: list[str] = field(default_factory=list)
    message: str = ""


def key_for_label(label: str) -> str | None:
    for pattern, key in FIELD_RULES:
        if re.search(pattern, label.strip(), re.I):
            return key
    return None


def fill_form(scope, applicant: Applicant) -> list[str]:
    """Fill text inputs and textareas in a page or frame by their label; returns the keys filled."""
    filled: list[str] = []
    for element in scope.query_selector_all("input:not([type=hidden]):not([type=file]):not([type=checkbox]):not([type=radio]):not([type=submit]), textarea"):
        try:
            if not element.is_visible() or element.input_value():
                continue
            label = element.evaluate(
                "e => (e.labels && e.labels[0] ? e.labels[0].innerText : '') || e.getAttribute('aria-label') || e.placeholder || e.name || e.id || ''"
            )
            key = key_for_label(str(label))
            value = applicant.value(key) if key else ""
            if key and value and key not in filled:
                element.fill(value)
                filled.append(key)
        except Exception:  # noqa: BLE001 - one odd field must not stop the rest
            continue
    return filled


def open_and_prefill(url: str, applicant: Applicant, on_ready: Callable[[ApplyResult], None], on_error: Callable[[str], None]) -> None:
    """Runs in its own thread: prefill, report, then keep the window alive until the user closes it."""
    try:
        from playwright.sync_api import sync_playwright

        assert_public_url(url)
        with sync_playwright() as playwright:
            profile = profile_path()
            profile.mkdir(parents=True, exist_ok=True)
            context = playwright.chromium.launch_persistent_context(
                str(profile), headless=False, user_agent=USER_AGENT, args=PROFILE_ARGS, ignore_default_args=["--use-mock-keychain"]
            )
            try:
                page = context.pages[0] if context.pages else context.new_page()
                page.goto(url, wait_until="domcontentloaded", timeout=30_000)
                try:
                    page.wait_for_load_state("networkidle", timeout=6_000)
                except Exception:  # noqa: BLE001
                    pass
                # Many boards show the posting first and the form behind an Apply button or iframe.
                for name in ("Apply for this job", "Apply now", "Apply"):
                    button = page.get_by_role("link", name=re.compile(rf"^{name}", re.I)).first
                    if button.count() and not page.query_selector("input[type=email], input[name*=email i]"):
                        button.click(timeout=3_000)
                        page.wait_for_load_state("domcontentloaded", timeout=10_000)
                        break
                filled: list[str] = []
                for scope in [page, *page.frames]:
                    for key in fill_form(scope, applicant):
                        if key not in filled:
                            filled.append(key)
                on_ready(ApplyResult(filled=filled))
                waited = 0
                while context.pages and waited < MAX_OPEN_SECONDS:
                    page.wait_for_timeout(1000) if not page.is_closed() else threading.Event().wait(1)
                    waited += 1
            finally:
                context.close()
    except ScrapeError as error:
        on_error(str(error))
    except Exception as error:  # noqa: BLE001 - browser missing, window closed early, etc.
        logging.getLogger("cortex.apply").exception("apply failed")
        on_error(f"could not open the application form ({type(error).__name__})")


_quick_lock = threading.Lock()


def open_quick(url: str, applicant: Applicant, wait_seconds: float = 45.0) -> dict:
    """Open and pre-fill a job's form right away, without drafting first. The window stays open for the person."""
    if not _quick_lock.acquire(blocking=False):
        raise ValueError("an application form is already being opened; finish or close it first")
    outcome: dict = {}
    done = threading.Event()

    def ready(result: ApplyResult) -> None:
        outcome.update(status="opened", filled=result.filled)
        done.set()

    def failed(message: str) -> None:
        outcome.update(status="failed", message=message, filled=[])
        done.set()

    def run() -> None:
        try:
            open_and_prefill(url, applicant, ready, failed)
        finally:
            done.set()
            _quick_lock.release()

    threading.Thread(target=run, daemon=True, name="cortex-quick-apply").start()
    if not done.wait(wait_seconds):
        return {"status": "opening", "filled": [], "message": "The form is still loading in the browser window."}
    return outcome
