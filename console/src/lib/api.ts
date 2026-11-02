export async function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  return readAPIResponse<T>(response, path);
}

export async function readAPIResponse<T>(response: Response, requestPath: string): Promise<T> {
  const body = await response.text();
  const endpoint = requestPath.split("?")[0];
  let payload: ({ error?: string } & T) | undefined;

  if (body.trim()) {
    try {
      payload = JSON.parse(body) as { error?: string } & T;
    } catch {
      if (!response.ok) {
        throw new Error(`API request to ${endpoint} failed (${response.status}) with a non-JSON response.`);
      }
      throw new Error(`API request to ${endpoint} returned invalid JSON (${response.status}).`);
    }
  }

  if (!response.ok) {
    throw new Error(payload?.error ?? `API request to ${endpoint} failed (${response.status}).`);
  }
  if (!payload) {
    throw new Error(`API request to ${endpoint} returned an empty response (${response.status}).`);
  }
  return payload;
}
