import { useState } from "react";
import { FilePenLine, Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { Button } from "@/components/ui/button";
import { MemoryCorrectDialog } from "@/features/memory/memory-correct-dialog";
import { MemoryDisableDialog } from "@/features/memory/memory-disable-dialog";
import { MemoryListPanel } from "@/features/memory/memory-list-panel";
import { MemorySuggestionReviewDialog } from "@/features/memory/memory-suggestion-review-dialog";
import {
  memoryStatusForView,
  type MemoryView,
} from "@/features/memory/memory-view";
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
  const [view, setView] = useState<MemoryView>("active");
  const [composing, setComposing] = useState(false);
  const [composeStarted, setComposeStarted] = useState(false);
  const [draftDirty, setDraftDirty] = useState(false);
  const [correcting, setCorrecting] = useState<MemoryAssertion | null>(null);
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

  function changeView(next: MemoryView) {
    setView(next);
    exitComposing();
    if (next !== "suggested") {
      setFilters((current) => ({
        ...current,
        status: memoryStatusForView(next),
      }));
    }
  }

  function startComposing() {
    setComposeStarted(true);
    setComposing(true);
  }

  function finishComposing() {
    setComposeStarted(false);
    setComposing(false);
    setDraftDirty(false);
    changeView("active");
  }

  function exitComposing() {
    setComposing(false);
    if (!draftDirty) setComposeStarted(false);
  }

  function discardComposing() {
    setComposeStarted(false);
    setComposing(false);
    setDraftDirty(false);
  }

  const hasDraft = composeStarted && draftDirty;

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.memory}
        title={t("memory.title")}
        description={t("memory.subtitle")}
      >
        {canPublish && (
          <Button type="button" size="sm" onClick={startComposing}>
            {hasDraft ? (
              <FilePenLine className="size-4" aria-hidden />
            ) : (
              <Plus className="size-4" aria-hidden />
            )}
            {t(hasDraft ? "memory.continueDraft" : "memory.record")}
          </Button>
        )}
      </PageHeader>

      <MemoryListPanel
        filters={filters}
        onFilters={setFilters}
        state={{ view, composing, composeStarted }}
        queries={{ assertions, suggestions }}
        canPublish={canPublish}
        onView={changeView}
        onComposeExit={exitComposing}
        onComposeDone={finishComposing}
        onComposeDiscard={discardComposing}
        onComposeDirty={setDraftDirty}
        onCorrect={setCorrecting}
        onDisable={setDisabling}
        onAccept={(suggestion) => setReviewing({ suggestion, action: "accept" })}
        onDismiss={(suggestion) => setReviewing({ suggestion, action: "dismiss" })}
      />

      {correcting && (
        <MemoryCorrectDialog
          assertion={correcting}
          onClose={() => setCorrecting(null)}
        />
      )}
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
