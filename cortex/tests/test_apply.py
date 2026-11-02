import pytest

from cortex.apply import Applicant, fill_form, key_for_label

FORM = """<form><label for=a>First Name *</label><input id=a>
<label for=b>Last Name</label><input id=b><label for=c>Email</label><input id=c type=email>
<label for=d>Phone</label><input id=d><label for=e>Cover Letter</label><textarea id=e></textarea>
<label for=f>Favourite colour</label><input id=f><input type=submit value=Submit></form>"""


def test_label_rules():
    assert key_for_label("First Name *") == "first_name"
    assert key_for_label("Email address") == "email"
    assert key_for_label("Why do you want this job?") is None


def test_fill_form_fills_known_fields_and_never_submits():
    sync_api = pytest.importorskip("playwright.sync_api")
    try:
        with sync_api.sync_playwright() as p:
            browser = p.chromium.launch(headless=True)
            page = browser.new_page()
            page.set_content(FORM)
            page.evaluate("document.forms[0].addEventListener('submit', () => { window.submitted = true })")
            filled = fill_form(page, Applicant("Ada Lovelace", "ada@example.com", "123", "", "Hello team"))
            assert page.input_value("#a") == "Ada" and page.input_value("#b") == "Lovelace"
            assert page.input_value("#c") == "ada@example.com" and page.input_value("#e") == "Hello team"
            assert page.input_value("#f") == "" and not page.evaluate("window.submitted")
            assert {"first_name", "email", "cover_note"} <= set(filled)
            browser.close()
    except Exception as error:  # noqa: BLE001
        if "Executable doesn't exist" in str(error):
            pytest.skip("chromium not installed")
        raise


def test_apply_requires_approved_draft():
    from fastapi.testclient import TestClient

    from cortex.embeddings import HashingEmbedder
    from cortex.llm import NoLLM
    from cortex.memory import Memory
    from cortex.server import create_app

    app = create_app(embedder=HashingEmbedder(), llm=NoLLM(), memory=Memory(HashingEmbedder()), use_browser=False)
    client = TestClient(app)
    assert client.post("/v1/applications/nope/apply", json={}).status_code == 404
