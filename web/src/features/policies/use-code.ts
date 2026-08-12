import { useState } from "react";
import type { Policy } from "@/lib/api/client";

/**
 * The code a new rule gets, pre-filled with the next one free.
 *
 * Pre-filled rather than assigned, because a code is a name people say out
 * loud and somebody may want POL-500 for a family of rules. Set once: after
 * that it is in the trail and in support conversations, and a code that moved
 * would orphan every one of them.
 */
export function useCode(
  creating: boolean,
  existing: Policy[],
  routeCode?: string,
) {
  const [code, setCode] = useState(() =>
    creating ? nextCode(existing) : (routeCode ?? ""),
  );
  return { code, setCode };
}

/** The lowest POL-NNN nobody is using, from 100 up. */
export function nextCode(existing: Policy[]): string {
  const taken = new Set(existing.map((p) => p.code));
  for (let n = 100; n < 1000; n += 1) {
    const candidate = `POL-${n}`;
    if (!taken.has(candidate)) return candidate;
  }
  return "POL-1000";
}
