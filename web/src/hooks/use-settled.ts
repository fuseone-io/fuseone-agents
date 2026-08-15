import { useEffect, useState } from "react";

/**
 * A value once it has stopped changing.
 *
 * For work that is worth doing about what somebody wrote but not worth doing
 * about every keystroke: a request per character is a request the answer to
 * which is stale before it paints, and it charges the typing speed of the
 * author to whatever is on the other end.
 *
 * The first value is settled immediately. What arrives already still — a
 * version loading, a template chosen — has nothing to wait for.
 */
export function useSettled<T>(value: T, afterMs: number): T {
  const [settled, setSettled] = useState(value);

  useEffect(() => {
    if (value === settled) return;
    const timer = window.setTimeout(() => setSettled(value), afterMs);
    return () => window.clearTimeout(timer);
  }, [value, settled, afterMs]);

  return settled;
}
