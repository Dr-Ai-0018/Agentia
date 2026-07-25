import { useCallback, useEffect, useState } from "react";

// Console-wide dev-mode toggle. Persists in localStorage so refreshes and new
// tabs pick it up. Cross-tab sync via the storage event; in-tab sync via a
// custom event dispatched on every write.
//
// Currently gates: the operator-only summary pane inline display inside
// ResidentsPage. Residents themselves never see any of it.

const STORAGE_KEY = "arena.console.devMode";
const CHANGE_EVENT = "arena:devMode:change";

function readDevMode(): boolean {
  if (typeof window === "undefined") return false;
  try {
    return window.localStorage.getItem(STORAGE_KEY) === "1";
  } catch {
    return false;
  }
}

function writeDevMode(next: boolean): void {
  if (typeof window === "undefined") return;
  try {
    if (next) {
      window.localStorage.setItem(STORAGE_KEY, "1");
    } else {
      window.localStorage.removeItem(STORAGE_KEY);
    }
    window.dispatchEvent(new CustomEvent(CHANGE_EVENT));
  } catch {
    // localStorage unavailable — no-op; hook state falls back to default.
  }
}

export function useDevMode(): [boolean, (next: boolean) => void] {
  const [enabled, setEnabled] = useState<boolean>(readDevMode);

  useEffect(() => {
    const handler = () => setEnabled(readDevMode());
    window.addEventListener(CHANGE_EVENT, handler);
    window.addEventListener("storage", handler);
    return () => {
      window.removeEventListener(CHANGE_EVENT, handler);
      window.removeEventListener("storage", handler);
    };
  }, []);

  const setDevMode = useCallback((next: boolean) => writeDevMode(next), []);

  return [enabled, setDevMode];
}
