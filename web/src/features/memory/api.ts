import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { useSettled } from "@/hooks/use-settled";
import { useScopeFilter } from "@/features/scope/use-scope-filter";
import type {
  MemoryAssertion,
  MemoryAssertionInput,
  MemorySuggestion,
  MemorySuggestionStatus,
  MemoryStatus,
  MemoryMatch,
  MemoryMatchInput,
} from "@/lib/api/client";

export type {
  MemoryAssertion,
  MemoryAssertionInput,
  MemorySuggestion,
  MemorySuggestionStatus,
  MemoryStatus,
  MemoryMatch,
  MemoryMatchInput,
};

export type MemoryStatusFilter = MemoryStatus | "all";
export type MemorySuggestionStatusFilter = MemorySuggestionStatus | "all";

export interface MemoryFilters {
  status: MemoryStatusFilter;
  search: string;
  agentId: string;
}

export interface MemorySuggestionFilters {
  status: MemorySuggestionStatusFilter;
  search: string;
  agentId: string;
}

export const memoryKeys = {
  all: ["memory"] as const,
  /** Keyed on the identity itself: the answer is about this fact, and two
   *  spellings of one identity are two questions until the server folds them. */
  match: (input: MemoryMatchInput) =>
    [
      ...memoryKeys.all,
      "match",
      input.company,
      input.area,
      input.namespace,
      input.agentId ?? "",
      input.kind,
      input.subject,
      input.signature,
    ] as const,
  list: (scope: string, filters: MemoryFilters) =>
    [
      ...memoryKeys.all,
      "list",
      scope,
      filters.status,
      filters.search.trim(),
      filters.agentId.trim(),
    ] as const,
  suggestions: (scope: string, filters: MemorySuggestionFilters) =>
    [
      ...memoryKeys.all,
      "suggestions",
      scope,
      filters.status,
      filters.search.trim(),
      filters.agentId.trim(),
    ] as const,
};

export function useMemoryAssertions(filters: MemoryFilters) {
  const scope = useScopeFilter();
  return useQuery({
    queryKey: memoryKeys.list(scope.key, filters),
    queryFn: async () =>
      unwrap(
        await api.GET("/admin/memory/assertions", {
          params: { query: { ...scope.params, ...queryOf(filters), limit: 100 } },
        }),
      ),
  });
}

export function useMemorySuggestions(filters: MemorySuggestionFilters) {
  const scope = useScopeFilter();
  return useQuery({
    queryKey: memoryKeys.suggestions(scope.key, filters),
    queryFn: async () =>
      unwrap(
        await api.GET("/admin/memory/suggestions", {
          params: { query: { ...scope.params, ...suggestionQueryOf(filters), limit: 100 } },
        }),
      ),
  });
}

export function useCreateMemoryAssertion() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: MemoryAssertionInput) =>
      unwrap(await api.POST("/admin/memory/assertions", { body })),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: memoryKeys.all }),
  });
}

export function useAcceptMemorySuggestion() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      company: string;
      area: string;
      reason: string;
      /**
       * The claim in better words, when somebody rewrote it. Omitted means
       * they agreed with the wording as well as with the fact — which is a
       * different thing to record, so the field is left out rather than sent
       * as the text that was already there.
       */
      claim?: string;
    }) =>
      unwrap(
        await api.POST("/admin/memory/suggestions/{suggestionId}/accept", {
          params: { path: { suggestionId: input.id } },
          body: {
            company: input.company,
            area: input.area,
            reason: input.reason,
            ...(input.claim === undefined ? {} : { claim: input.claim }),
          },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: memoryKeys.all }),
  });
}

export function useDismissMemorySuggestion() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      company: string;
      area: string;
      reason: string;
    }) =>
      unwrap(
        await api.POST("/admin/memory/suggestions/{suggestionId}/dismiss", {
          params: { path: { suggestionId: input.id } },
          body: {
            company: input.company,
            area: input.area,
            reason: input.reason,
          },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: memoryKeys.all }),
  });
}

/**
 * Bringing back a memory somebody disabled.
 *
 * A separate act from creating one, and deliberately not reachable by
 * re-teaching the same fact: the server refuses to merge into a disabled row,
 * so without this the only way past a disabled memory would be to teach a
 * different one and leave two records of the same thing.
 */
export function useReactivateMemoryAssertion() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      company: string;
      area: string;
      reason: string;
    }) =>
      unwrap(
        await api.POST("/admin/memory/assertions/{assertionId}/reactivate", {
          params: { path: { assertionId: input.id } },
          body: {
            company: input.company,
            area: input.area,
            reason: input.reason,
          },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: memoryKeys.all }),
  });
}

/**
 * What the platform already holds about this identity.
 *
 * Asked while somebody types, so it is debounced: the identity is not a fact
 * until they stop, and a request per keystroke would ask the server about
 * prefixes nobody means. Four hundred milliseconds is long enough that the
 * common case is one question and short enough that the answer arrives before
 * the decision.
 *
 * Disabled until the identity is complete. An incomplete one has no answer —
 * the server refuses it — and asking anyway would spend the round trip to be
 * told what the form already knows. Callers may also hold the query while
 * resolving provenance the match request must carry, such as the run's agent.
 */
export function useMemoryMatch(
  input: MemoryMatchInput,
  options: { enabled?: boolean; settle?: boolean } = {},
) {
  const { enabled = true, settle = true } = options;
  // Settled on the identity as one string rather than on the object: a new
  // object every render never equals the last, so waiting on the object itself
  // would wait for ever and ask nothing.
  const current = memoryKeys.match(input).join("\u0000");
  const delayed = useSettled(current, 400);
  const settled = settle ? delayed : current;
  const asked = matchFromKey(settled);
  const complete = Boolean(
    asked.company && asked.area && asked.kind && asked.subject && asked.signature,
  );
  const namespaceComplete = asked.namespace === "shared" || Boolean(asked.agentId);
  const isSettling = current !== settled;
  const query = useQuery({
    queryKey: memoryKeys.match(asked),
    enabled: enabled && !isSettling && complete && namespaceComplete,
    queryFn: async () =>
      unwrap(await api.POST("/admin/memory/match", { body: asked })),
  });
  return {
    ...query,
    isSettling,
  };
}

/**
 * The identity back out of its key.
 *
 * The key is what settles, so it is also what the request has to be built from
 * — reading the live input here instead would ask about the identity the person
 * is typing now while the query is filed under the one they had stopped at, and
 * the answer would be cached against the wrong fact.
 */
function matchFromKey(key: string): MemoryMatchInput {
  const [, , company, area, namespace, agentId, kind, subject, signature] =
    key.split("\u0000");
  return {
    company: company ?? "",
    area: area ?? "",
    namespace: (namespace ?? "agent") as MemoryMatchInput["namespace"],
    agentId: agentId || undefined,
    kind: kind ?? "",
    subject: subject ?? "",
    signature: signature ?? "",
  };
}

export function useDisableMemoryAssertion() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      company: string;
      area: string;
      reason: string;
    }) =>
      unwrap(
        await api.POST("/admin/memory/assertions/{assertionId}/disable", {
          params: { path: { assertionId: input.id } },
          body: {
            company: input.company,
            area: input.area,
            reason: input.reason,
          },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: memoryKeys.all }),
  });
}

function queryOf(filters: MemoryFilters) {
  return {
    status: filters.status === "all" ? undefined : filters.status,
    q: clean(filters.search),
    agentId: clean(filters.agentId),
  };
}

function suggestionQueryOf(filters: MemorySuggestionFilters) {
  return {
    status: filters.status === "all" ? undefined : filters.status,
    q: clean(filters.search),
    agentId: clean(filters.agentId),
  };
}

function clean(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}
