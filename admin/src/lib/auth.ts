const TOKEN_KEY = "lynx_token";
let cachedToken: string | null = null;

export async function getToken(): Promise<string> {
  if (cachedToken) return cachedToken;

  const stored = typeof window !== "undefined" ? localStorage.getItem(TOKEN_KEY) : null;
  if (stored) {
    cachedToken = stored;
    return cachedToken;
  }

  try {
    const res = await fetch("/api/token");
    if (res.ok) {
      const token = await res.text();
      if (typeof window !== "undefined") localStorage.setItem(TOKEN_KEY, token);
      cachedToken = token;
      return token;
    }
  } catch {
    // network error — return empty, requests will 401 and UI will handle it
  }

  return "";
}

export async function authedFetch(url: string, options?: RequestInit): Promise<Response> {
  const token = await getToken();
  return fetch(url, {
    ...options,
    headers: {
      ...options?.headers,
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
  });
}
