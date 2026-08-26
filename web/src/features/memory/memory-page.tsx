import { useState } from "react";
import { useTranslation } from "react-i18next";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { Panel } from "@/components/shared/panel";
import { MemoryCreatePanel } from "@/features/memory/memory-create-panel";
import { MemoryDisableDialog } from "@/features/memory/memory-disable-dialog";
import { MemoryListPanel } from "@/features/memory/memory-list-panel";
import { MemorySuggestionReviewDialog } from "@/features/memory/memory-suggestion-review-dialog";
import { MemorySuggestionsPanel } from "@/features/memory/memory-suggestions-panel";
import {
  useMemoryAssertions,
  useMemorySuggestions,
  type MemoryAssertion,
  type MemorySuggestion,
} from "@/features/memory/api";
import { useMe } from "@/features/session/api";
import type { MemoryFilters } from "@/features/memory/api";

const DEFAULT_FILTERS: MemoryFilters = {
  status: "active",
  search: "",
  agentId: "",
};

export function MemoryPage() {
  const { t } = useTranslation();
  const [filters, setFilters] = useState(DEFAULT_FILTERS);
  const [disabling, setDisabling] = useState<MemoryAssertion | null>(null);
  const [reviewing, setReviewing] = useState<{
    suggestion: MemorySuggestion;
    action: "accept" | "dismiss";
  } | null>(null);
  const assertions = useMemoryAssertions(filters);
  const suggestions = useMemorySuggestions({
    status: "pending",
    search: filters.search,
    agentId: filters.agentId,
  });
  const { data: me } = useMe();
  const canPublish = me === null || Boolean(me?.can.includes("agent:publish"));

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.memory}
        title={t("memory.title")}
        description={t("memory.subtitle")}
      />

      <div className="grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(320px,380px)]">
        <MemoryListPanel
          filters={filters}
          onFilters={setFilters}
          query={assertions}
          canDisable={canPublish}
          onDisable={setDisabling}
        />
        <div className="grid min-w-0 content-start gap-4">
          <MemorySuggestionsPanel
            query={suggestions}
            canReview={canPublish}
            onAccept={(suggestion) => setReviewing({ suggestion, action: "accept" })}
            onDismiss={(suggestion) => setReviewing({ suggestion, action: "dismiss" })}
          />
          {canPublish ? <MemoryCreatePanel /> : <MemoryReadOnlyPanel />}
        </div>
      </div>

      {disabling && (
        <MemoryDisableDialog
          assertion={disabling}
          onClose={() => setDisabling(null)}
        />
      )}
      {reviewing && (
        <MemorySuggestionReviewDialog
          suggestion={reviewing.suggestion}
          action={reviewing.action}
          onClose={() => setReviewing(null)}
        />
      )}
    </>
  );
}

function MemoryReadOnlyPanel() {
  const { t } = useTranslation();
  return (
    <Panel title={t("memory.readOnlyTitle")}>
      <p className="text-sm text-muted-foreground">
        {t("memory.readOnlyHint")}
      </p>
    </Panel>
  );
}
