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

  // Anything but a caller is simply nobody, and that is an answer rather than
  // a failure: 401 on a protected installation, 404 on one running with no
  // identity at all. Throwing put the console into its error state and left an
  // errored query refetching on every render.
  if (!response.ok) return null;

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
