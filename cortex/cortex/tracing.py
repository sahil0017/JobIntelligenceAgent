"""Optional tracing of the LangGraph workflow to a local Arize Phoenix.

Enabled only when PHOENIX_COLLECTOR_ENDPOINT is set and the packages from
requirements-observability.txt are installed. Trace spans contain job text and résumé passages,
so the endpoint must be a loopback address unless CORTEX_TRACING_ALLOW_REMOTE=1.
"""

from __future__ import annotations

import logging
import os
from urllib.parse import urlparse

log = logging.getLogger("cortex.tracing")
_LOOPBACK = {"127.0.0.1", "localhost", "::1"}


def setup_tracing() -> bool:
    endpoint = os.environ.get("PHOENIX_COLLECTOR_ENDPOINT", "").strip()
    if not endpoint:
        return False
    host = urlparse(endpoint).hostname or ""
    if host not in _LOOPBACK and os.environ.get("CORTEX_TRACING_ALLOW_REMOTE") != "1":
        log.warning("tracing disabled: %s is not a local address (set CORTEX_TRACING_ALLOW_REMOTE=1 to allow)", host)
        return False
    try:
        from openinference.instrumentation.langchain import LangChainInstrumentor
        from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
        from opentelemetry.sdk.trace import TracerProvider
        from opentelemetry.sdk.trace.export import BatchSpanProcessor
    except ImportError:
        log.warning("tracing disabled: run `pip install -r cortex/requirements-observability.txt`")
        return False
    provider = TracerProvider()
    provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter(endpoint=endpoint.rstrip("/") + "/v1/traces")))
    LangChainInstrumentor().instrument(tracer_provider=provider)
    return True
