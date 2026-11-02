import { useCallback, useEffect, useState } from "react";
import { apiRequest } from "../../lib/api";
import { AIRun, AIStatus } from "./types";

const POLL_MS = 800;

export type RunKindPath = "analyses" | "applications";

/** Polls one run until it leaves the "running" state; reload() fetches it again after a decision. */
export function useRun(runId: string | null, path: RunKindPath = "analyses"): { run: AIRun | null; error: string; reload: () => void } {
  const [run, setRun] = useState<AIRun | null>(null);
  const [error, setError] = useState("");
  const [nonce, setNonce] = useState(0);
  const reload = useCallback(() => setNonce((value) => value + 1), []);

  useEffect(() => {
    if (nonce === 0) setRun(null);
    setError("");
    if (!runId) return;
    let cancelled = false;
    let timer: number | undefined;
    const tick = async () => {
      try {
        const next = await apiRequest<AIRun>(`/api/ai/${path}/${encodeURIComponent(runId)}`);
        if (cancelled) return;
        setRun(next);
        setError("");
        if (next.status === "running") timer = window.setTimeout(() => void tick(), POLL_MS);
      } catch (reason) {
        if (cancelled) return;
        setError(reason instanceof Error ? reason.message : "Could not load the analysis run.");
        timer = window.setTimeout(() => void tick(), POLL_MS * 4);
      }
    };
    void tick();
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [runId, path, nonce]);

  return { run: run && run.id === runId ? run : null, error, reload };
}

export function useAIStatus(): AIStatus | null {
  const [status, setStatus] = useState<AIStatus | null>(null);
  useEffect(() => {
    let cancelled = false;
    apiRequest<AIStatus>("/api/ai/status")
      .then((value) => !cancelled && setStatus(value))
      .catch(() => !cancelled && setStatus({ available: false, message: "Could not reach the platform API." }));
    return () => {
      cancelled = true;
    };
  }, []);
  return status;
}

export function formatDuration(ms: number): string {
  if (ms <= 0) return "";
  return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`;
}
