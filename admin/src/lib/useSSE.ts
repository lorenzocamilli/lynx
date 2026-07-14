import { useEffect, useRef } from "react";

import { getToken } from "./auth";

export function useSSE(onMessage: (type: string) => void, debounceMs = 0): void {
  const ref = useRef(onMessage);
  ref.current = onMessage;

  useEffect(() => {
    let es: EventSource | null = null;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let closed = false;

    // EventSource can't set an Authorization header, so the token is passed as
    // a query param (the server validates it against the admin token).
    getToken().then((token) => {
      if (closed) return;

      es = new EventSource(`/api/events?token=${encodeURIComponent(token)}`);
      es.onmessage = (e: MessageEvent) => {
        try {
          const { type } = JSON.parse(e.data as string) as { type: string };
          if (debounceMs <= 0) {
            ref.current(type);
          } else {
            if (timer !== null) clearTimeout(timer);
            timer = setTimeout(() => ref.current(type), debounceMs);
          }
        } catch {
          // ignore malformed events
        }
      };
    });

    return () => {
      closed = true;
      if (timer !== null) clearTimeout(timer);
      if (es !== null) es.close();
    };
  }, [debounceMs]);
}
