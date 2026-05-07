import { useEffect, useRef } from "react";

export function useSSE(onMessage: (type: string) => void, debounceMs = 0): void {
  const ref = useRef(onMessage);
  ref.current = onMessage;

  useEffect(() => {
    const es = new EventSource("/api/events");
    let timer: ReturnType<typeof setTimeout> | null = null;

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

    return () => {
      if (timer !== null) clearTimeout(timer);
      es.close();
    };
  }, [debounceMs]);
}
