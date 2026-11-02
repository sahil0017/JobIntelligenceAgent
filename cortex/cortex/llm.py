"""Language-model backends for the writing agents.

Ollama (local) is the default so résumé text never leaves the machine. OpenAI and Anthropic are
opt-in through CORTEX_LLM and their API-key variables; `remote` tells the UI when data leaves.
`NoLLM` makes the agents fall back to deterministic, résumé-grounded templates.
"""

from __future__ import annotations

import os
from typing import Protocol

import httpx

TIMEOUT = httpx.Timeout(120.0, connect=3.0)
# A local CPU model answers parallel agents one at a time, so give it longer to queue.
LOCAL_TIMEOUT = httpx.Timeout(420.0, connect=3.0)


class LLMError(RuntimeError):
    pass


class LLM(Protocol):
    name: str
    remote: bool

    def complete(self, system: str, prompt: str, max_tokens: int = 700) -> str: ...


class NoLLM:
    name = "none (templates)"
    remote = False

    def complete(self, system: str, prompt: str, max_tokens: int = 700) -> str:
        raise LLMError("no language model configured")


class OllamaLLM:
    remote = False

    def __init__(self, base_url: str, model: str):
        self.base_url = base_url.rstrip("/")
        self.model = model
        self.name = f"ollama:{model}"

    def complete(self, system: str, prompt: str, max_tokens: int = 700) -> str:
        try:
            response = httpx.post(
                f"{self.base_url}/api/chat",
                json={
                    "model": self.model,
                    "stream": False,
                    "messages": [{"role": "system", "content": system}, {"role": "user", "content": prompt}],
                    "keep_alive": os.environ.get("OLLAMA_KEEP_ALIVE", "30m"),
                    "options": {"num_predict": max_tokens, "temperature": 0.3, "num_ctx": int(os.environ.get("OLLAMA_NUM_CTX", "2560"))},
                },
                timeout=LOCAL_TIMEOUT,
            )
            response.raise_for_status()
            return str(response.json()["message"]["content"]).strip()
        except (httpx.HTTPError, KeyError, ValueError) as error:
            raise LLMError(f"Ollama request failed: {error}") from error


def _http_reason(error: Exception) -> str:
    """Status and OpenAI error code only, so failures are diagnosable without echoing any message text."""
    if isinstance(error, httpx.HTTPStatusError):
        try:
            code = error.response.json().get("error", {}).get("code") or ""
        except ValueError:
            code = ""
        return f"HTTP {error.response.status_code}" + (f" ({code})" if isinstance(code, str) and code else "")
    return type(error).__name__


class OpenAILLM:
    remote = True

    def __init__(self, api_key: str, model: str, base_url: str = "https://api.openai.com/v1"):
        self.api_key, self.model, self.base_url = api_key, model, base_url.rstrip("/")
        self.name = f"openai:{model}"

    def complete(self, system: str, prompt: str, max_tokens: int = 700) -> str:
        try:
            response = httpx.post(
                f"{self.base_url}/chat/completions",
                headers={"Authorization": f"Bearer {self.api_key}"},
                json={
                    "model": self.model,
                    "max_tokens": max_tokens,
                    "temperature": 0.3,
                    "messages": [{"role": "system", "content": system}, {"role": "user", "content": prompt}],
                },
                timeout=TIMEOUT,
            )
            response.raise_for_status()
            return str(response.json()["choices"][0]["message"]["content"]).strip()
        except (httpx.HTTPError, KeyError, IndexError, ValueError) as error:
            raise LLMError(f"OpenAI request failed: {_http_reason(error)}") from error


    def research(self, query: str, max_tokens: int = 500) -> str:
        """Live web search through the Responses API. Only `query` leaves the machine."""
        try:
            response = httpx.post(
                f"{self.base_url}/responses",
                headers={"Authorization": f"Bearer {self.api_key}"},
                json={
                    "model": self.model,
                    "tools": [{"type": "web_search"}],
                    "max_output_tokens": max_tokens,
                    "input": query,
                },
                timeout=httpx.Timeout(90.0, connect=10.0),
            )
            response.raise_for_status()
            parts = [
                block.get("text", "")
                for item in response.json().get("output", []) if item.get("type") == "message"
                for block in item.get("content", []) if block.get("type") == "output_text"
            ]
            return "\n".join(parts).strip()
        except (httpx.HTTPError, ValueError) as error:
            raise LLMError(f"OpenAI web search failed: {_http_reason(error)}") from error


class AnthropicLLM:
    remote = True

    def __init__(self, api_key: str, model: str):
        self.api_key, self.model = api_key, model
        self.name = f"anthropic:{model}"

    def complete(self, system: str, prompt: str, max_tokens: int = 700) -> str:
        try:
            response = httpx.post(
                "https://api.anthropic.com/v1/messages",
                headers={"x-api-key": self.api_key, "anthropic-version": "2023-06-01"},
                json={
                    "model": self.model,
                    "max_tokens": max_tokens,
                    "temperature": 0.3,
                    "system": system,
                    "messages": [{"role": "user", "content": prompt}],
                },
                timeout=TIMEOUT,
            )
            response.raise_for_status()
            return "".join(block.get("text", "") for block in response.json()["content"]).strip()
        except (httpx.HTTPError, KeyError, ValueError) as error:
            raise LLMError(f"Anthropic request failed: {type(error).__name__}") from error


def build_llm(choice: str | None = None) -> LLM:
    """CORTEX_LLM: ollama, openai, anthropic or none. Defaults to openai when OPENAI_API_KEY is set, else ollama."""
    default = "openai" if os.environ.get("OPENAI_API_KEY") else "ollama"
    choice = (choice or os.environ.get("CORTEX_LLM", default)).strip().lower()
    model = os.environ.get("CORTEX_LLM_MODEL", "").strip()
    if choice == "ollama":
        return OllamaLLM(
            os.environ.get("OLLAMA_URL", "http://127.0.0.1:11434"),
            model or os.environ.get("OLLAMA_MODEL", "qwen2.5:7b"),
        )
    if choice == "openai":
        key = os.environ.get("OPENAI_API_KEY", "")
        if not key:
            raise ValueError("CORTEX_LLM=openai needs OPENAI_API_KEY")
        return OpenAILLM(key, model or "gpt-4o-mini", os.environ.get("OPENAI_BASE_URL", "https://api.openai.com/v1"))
    if choice == "anthropic":
        key = os.environ.get("ANTHROPIC_API_KEY", "")
        if not key:
            raise ValueError("CORTEX_LLM=anthropic needs ANTHROPIC_API_KEY")
        return AnthropicLLM(key, model or "claude-3-5-sonnet-latest")
    if choice in ("none", "off", ""):
        return NoLLM()
    raise ValueError(f"unknown CORTEX_LLM {choice!r}; use ollama, openai, anthropic or none")
