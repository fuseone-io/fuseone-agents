import { useCompanies } from "@/features/companies/api";
import { useScopes } from "@/features/scope/api";

/**
 * The companies and areas a grant can name.
 *
 * Both lists are read by the editor and by each row's selects; TanStack keeps
 * one request per list however many rows ask.
 */
export function useGrantScopes() {
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
