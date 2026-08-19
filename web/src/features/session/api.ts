import { useQuery } from "@tanstack/react-query";

/**
 * The session endpoints sit outside the OpenAPI contract on purpose: they are
 * browser redirects and cookie handling, not an API a CLI or CI job would
 * call. That is why this module is hand written rather than generated.
 */

export interface MeGrant {
  company: string;
  area: string;
  role: string;
}

export interface Me {
  id: string;
  display: string;
  kind: string;
  grants: MeGrant[];
  /**
   * Permissions the caller holds somewhere. A hint for the interface only —
   * every request is checked again on the server, where the scope of the
   * specific resource is known.
   */
  can: string[];
}

export const sessionKeys = { me: ["session", "me"] as const };

async function fetchMe(): Promise<Me | null> {
  const response = await fetch("/api/v1/me", { credentials: "same-origin" });

  // A missing caller is an answer only for the two shapes that mean it:
  // unsigned on a protected installation, or no identity configured at all.
  // A transient server error is not open mode; caching it as `null` would
  // briefly remove every permission filter in the console.
  if (response.status === 401 || response.status === 404) return null;
  if (!response.ok) {
    throw new Error(`session lookup failed with status ${response.status}`);
  }

  return (await response.json()) as Me;
}

export function useMe() {
  return useQuery({
    queryKey: sessionKeys.me,
    queryFn: fetchMe,
    staleTime: 60_000,
    retry: false,
  });
}
