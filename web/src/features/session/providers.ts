import { useQuery } from "@tanstack/react-query";
import { ApiError } from "@/lib/api/client";

export interface IdentityProvider {
  id: string;
  display: string;
}

export interface SignInOptions {
  providers: IdentityProvider[];
  /** True while nobody holds Curator anywhere: the installation is unclaimed. */
  bootstrapPending: boolean;
  /**
   * False when the server has no identity configured at all. Putting a
   * sign-in screen in front of it would be a lock on an open door: it stops
   * nobody and leaves the console unreachable.
   */
  authRequired: boolean;
  /** Whether anybody can sign in with a password at all. */
  localSignIn: boolean;
}

export const providerKeys = { all: ["auth", "providers"] as const };

/**
 * What the sign-in screen may offer, and whether this installation has been
 * claimed at all. Both answers come from one call so a fresh install shows the
 * setup screen rather than a login form with no providers on it.
 */
export function useSignInOptions() {
  return useQuery({
    queryKey: providerKeys.all,
    queryFn: async (): Promise<SignInOptions> => {
      const response = await fetch("/auth/providers", {
        credentials: "same-origin",
      });
      if (!response.ok) throw new ApiError(response.status);
      return (await response.json()) as SignInOptions;
    },
    staleTime: 30_000,
    retry: false,
  });
}
