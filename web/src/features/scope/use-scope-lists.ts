import { useCompanies } from "@/features/companies/api";
import { useScopes } from "@/features/scope/api";

/**
 * The companies and areas a company and area select can offer.
 *
 * Read by every select on a screen; TanStack keeps one request per list
 * however many of them ask.
 */
export function useScopeLists() {
  const companies = useCompanies();
  const scopes = useScopes();
  return {
    data: { companies: companies.data?.items, scopes: scopes.data?.items },
    isLoading: companies.isLoading || scopes.isLoading,
    error: companies.error ?? scopes.error,
    refetch: () => {
      void companies.refetch();
      void scopes.refetch();
    },
  };
}
