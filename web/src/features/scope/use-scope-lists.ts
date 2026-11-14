import { useCompanies } from "@/features/companies/api";
import { useScopes } from "@/features/scope/api";
import { ApiError } from "@/lib/api/client";

/**
 * The companies and areas a company and area select can offer.
 *
 * Read by every select on a screen; TanStack keeps one request per list
 * however many of them ask.
 *
 * The company list needs authority over the installation, which somebody
 * configuring at the platform may not hold while still placing things at any
 * scope. That refusal is expected, not a failure: `refused` says so, and it is
 * never the `error` a retry is offered for, since a retry would be refused
 * again.
 */
export function useScopeLists() {
  const companies = useCompanies();
  const scopes = useScopes();
  const refused =
    companies.error instanceof ApiError && companies.error.status === 403;
  return {
    data: { companies: companies.data?.items, scopes: scopes.data?.items },
    refused,
    isLoading: companies.isLoading || scopes.isLoading,
    error: (refused ? null : companies.error) ?? scopes.error,
    refetch: () => {
      void companies.refetch();
      void scopes.refetch();
    },
  };
}
