import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { useScopeFilter } from "@/features/scope/use-scope-filter";
import type {
  MemoryAssertion,
  MemoryAssertionInput,
  MemorySuggestion,
  MemorySuggestionStatus,
  MemoryStatus,
} from "@/lib/api/client";

export type {
  MemoryAssertion,
  MemoryAssertionInput,
  MemorySuggestion,
  MemorySuggestionStatus,
  MemoryStatus,
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
    }) =>
      unwrap(
        await api.POST("/admin/memory/suggestions/{suggestionId}/accept", {
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
