import { useMemo } from "react";
import {
  scopeKeyOf,
  scopeParamsOf,
  useActiveScope,
} from "@/features/scope/active-scope";

/**
 * The active context, in the two shapes every read needs it.
 *
 * `params` goes into the request; `key` goes into the query key. Both, always:
 * a request that carries the scope while its key does not will be answered
 * from the previous context's cache, so switching would appear to do nothing
 * until something else invalidated it.
 */
export function useScopeFilter(): {
  params: { company?: string; area?: string };
  key: string;
} {
  const company = useActiveScope((s) => s.company);
  const area = useActiveScope((s) => s.area);

  return useMemo(() => {
    const scope = { company, area };
    return { params: scopeParamsOf(scope), key: scopeKeyOf(scope) };
  }, [company, area]);
}
