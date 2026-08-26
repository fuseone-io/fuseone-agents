import type { UseQueryResult } from "@tanstack/react-query";
import { Lightbulb } from "lucide-react";
import { useTranslation } from "react-i18next";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { LoadMore } from "@/components/shared/load-more";
import { Panel } from "@/components/shared/panel";
import { MemorySuggestionCard } from "@/features/memory/memory-suggestion-card";
import type { MemorySuggestion } from "@/features/memory/api";
import { useVisibleItems } from "@/hooks/use-visible-items";

const PAGE_SIZE = 8;

export function MemorySuggestionsPanel({
  query,
  canReview,
  onAccept,
  onDismiss,
}: {
  query: UseQueryResult<{ items: MemorySuggestion[] }, Error>;
  canReview: boolean;
  onAccept: (suggestion: MemorySuggestion) => void;
  onDismiss: (suggestion: MemorySuggestion) => void;
}) {
  const { t } = useTranslation();
  const items = query.data?.items ?? [];
  const page = useVisibleItems(items, PAGE_SIZE);

  return (
    <Panel title={t("memory.suggestions")}>
      {query.isLoading ? (
        <LoadingRows rows={3} />
      ) : query.error ? (
        <ErrorState error={query.error} onRetry={() => void query.refetch()} />
      ) : page.visible.length === 0 ? (
        <EmptyState
          icon={<Lightbulb className="size-6" />}
          title={t("memory.noSuggestions")}
          hint={t("memory.noSuggestionsHint")}
        />
      ) : (
        <>
          <div className="grid min-w-0 gap-3">
            {page.visible.map((suggestion) => (
              <MemorySuggestionCard
                key={suggestion.id}
                suggestion={suggestion}
                canReview={canReview}
                onAccept={onAccept}
                onDismiss={onDismiss}
              />
            ))}
          </div>
          <LoadMore
            loaded={page.loaded}
            total={page.total}
            hasMore={page.hasMore}
            isLoading={false}
            onLoad={page.loadMore}
          />
        </>
      )}
    </Panel>
  );
}
