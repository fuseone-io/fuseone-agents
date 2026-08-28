import { useState, type ReactNode } from "react";
import type { UseQueryResult } from "@tanstack/react-query";
import { Lightbulb, Search } from "lucide-react";
import { useTranslation } from "react-i18next";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { LoadMore } from "@/components/shared/load-more";
import { Panel } from "@/components/shared/panel";
import { ScrollArea } from "@/components/ui/scroll-area";
import { MemoryAssertionCard } from "@/features/memory/memory-assertion-card";
import { MemoryAssertionIndex } from "@/features/memory/memory-assertion-index";
import { MemoryBrowserToolbar } from "@/features/memory/memory-browser-toolbar";
import { MemoryCreatePanel } from "@/features/memory/memory-create-panel";
import { MemorySuggestionCard } from "@/features/memory/memory-suggestion-card";
import { MemorySuggestionIndex } from "@/features/memory/memory-suggestion-index";
import type {
  MemoryAssertion,
  MemoryFilters,
  MemorySuggestion,
} from "@/features/memory/api";
import type { MemoryView } from "@/features/memory/memory-view";
import { useVisibleItems } from "@/hooks/use-visible-items";

const PAGE_SIZE = 8;

interface MemoryListPanelProps {
  filters: MemoryFilters;
  state: { view: MemoryView; composing: boolean; composeStarted: boolean };
  queries: {
    assertions: UseQueryResult<{ items: MemoryAssertion[] }, Error>;
    suggestions: UseQueryResult<{ items: MemorySuggestion[] }, Error>;
  };
  canPublish: boolean;
  onFilters: (filters: MemoryFilters) => void;
  onView: (view: MemoryView) => void;
  onComposeExit: () => void;
  onComposeDone: () => void;
  onComposeDiscard: () => void;
  onComposeDirty: (dirty: boolean) => void;
  onCorrect?: (assertion: MemoryAssertion) => void;
  onDisable: (assertion: MemoryAssertion) => void;
  onAccept: (suggestion: MemorySuggestion) => void;
  onDismiss: (suggestion: MemorySuggestion) => void;
}

export function MemoryListPanel(props: MemoryListPanelProps) {
  const { t } = useTranslation();
  const assertions = props.queries.assertions.data?.items ?? [];
  const suggestions = props.queries.suggestions.data?.items ?? [];
  const assertionPage = useVisibleItems(assertions, PAGE_SIZE);
  const suggestionPage = useVisibleItems(suggestions, PAGE_SIZE);
  const [selectedAssertionID, setSelectedAssertionID] = useState<string | null>(null);
  const [selectedSuggestionID, setSelectedSuggestionID] = useState<string | null>(null);
  const selectedAssertion =
    assertionPage.visible.find((item) => item.id === selectedAssertionID) ??
    assertionPage.visible[0] ??
    null;
  const selectedSuggestion =
    suggestionPage.visible.find((item) => item.id === selectedSuggestionID) ??
    suggestionPage.visible[0] ??
    null;

  function selectAssertion(id: string) {
    setSelectedAssertionID(id);
    props.onComposeExit();
  }

  function selectSuggestion(id: string) {
    setSelectedSuggestionID(id);
    props.onComposeExit();
  }

  return (
    <Panel title={t("memory.assertions")} flush>
      <MemoryBrowserToolbar
        filters={props.filters}
        view={props.state.view}
        onFilters={props.onFilters}
        onView={props.onView}
      />
      <div className="grid min-h-[620px] min-w-0 lg:grid-cols-[minmax(0,370px)_minmax(0,1fr)]">
        <aside className="min-h-0 min-w-0 border-b lg:border-r lg:border-b-0">
          <ScrollArea className="h-[620px]">
            <ListSide
              props={props}
              assertionPage={assertionPage}
              suggestionPage={suggestionPage}
              selectedAssertionID={selectedAssertion?.id}
              selectedSuggestionID={selectedSuggestion?.id}
              onAssertionSelect={selectAssertion}
              onSuggestionSelect={selectSuggestion}
            />
          </ScrollArea>
        </aside>
        <section className="min-h-0 min-w-0 overflow-y-auto p-4">
          <ReaderSide
            props={props}
            selectedAssertion={selectedAssertion}
            selectedSuggestion={selectedSuggestion}
          />
        </section>
      </div>
    </Panel>
  );
}

function ListSide({
  props,
  assertionPage,
  suggestionPage,
  selectedAssertionID,
  selectedSuggestionID,
  onAssertionSelect,
  onSuggestionSelect,
}: {
  props: MemoryListPanelProps;
  assertionPage: ReturnType<typeof useVisibleItems<MemoryAssertion>>;
  suggestionPage: ReturnType<typeof useVisibleItems<MemorySuggestion>>;
  selectedAssertionID?: string;
  selectedSuggestionID?: string;
  onAssertionSelect: (id: string) => void;
  onSuggestionSelect: (id: string) => void;
}) {
  if (props.state.view === "suggested") {
    return (
      <SuggestionList
        query={props.queries.suggestions}
        page={suggestionPage}
        selectedID={selectedSuggestionID}
        onSelect={(item) => onSuggestionSelect(item.id)}
      />
    );
  }
  return (
    <AssertionList
      query={props.queries.assertions}
      page={assertionPage}
      selectedID={selectedAssertionID}
      onSelect={(item) => onAssertionSelect(item.id)}
    />
  );
}

function ReaderSide({
  props,
  selectedAssertion,
  selectedSuggestion,
}: {
  props: MemoryListPanelProps;
  selectedAssertion: MemoryAssertion | null;
  selectedSuggestion: MemorySuggestion | null;
}) {
  return (
    <>
      {props.state.composeStarted && (
        <div className={props.state.composing ? undefined : "hidden"}>
          <MemoryCreatePanel
            framed={false}
            onExit={props.onComposeExit}
            onDone={props.onComposeDone}
            onDiscard={props.onComposeDiscard}
            onDirtyChange={props.onComposeDirty}
          />
        </div>
      )}
      {!props.state.composing && (
        <MemoryReader
          view={props.state.view}
          canPublish={props.canPublish}
          selection={{ assertion: selectedAssertion, suggestion: selectedSuggestion }}
          actions={{
            correct: props.onCorrect,
            disable: props.onDisable,
            accept: props.onAccept,
            dismiss: props.onDismiss,
          }}
        />
      )}
    </>
  );
}

function MemoryReader({
  view,
  canPublish,
  selection,
  actions,
}: {
  view: MemoryView;
  canPublish: boolean;
  selection: {
    assertion: MemoryAssertion | null;
    suggestion: MemorySuggestion | null;
  };
  actions: {
    correct?: (assertion: MemoryAssertion) => void;
    disable: (assertion: MemoryAssertion) => void;
    accept: (suggestion: MemorySuggestion) => void;
    dismiss: (suggestion: MemorySuggestion) => void;
  };
}) {
  if (view === "suggested") {
    if (!selection.suggestion) return <ReaderEmpty kind="suggestions" />;
    return (
      <MemorySuggestionCard
        suggestion={selection.suggestion}
        canReview={canPublish}
        onAccept={actions.accept}
        onDismiss={actions.dismiss}
      />
    );
  }
  if (!selection.assertion) return <ReaderEmpty kind="assertions" />;
  return (
    <MemoryAssertionCard
      assertion={selection.assertion}
      canDisable={canPublish}
      onCorrect={actions.correct}
      onDisable={actions.disable}
    />
  );
}

function AssertionList({
  query,
  page,
  selectedID,
  onSelect,
}: {
  query: UseQueryResult<{ items: MemoryAssertion[] }, Error>;
  page: ReturnType<typeof useVisibleItems<MemoryAssertion>>;
  selectedID?: string;
  onSelect: (assertion: MemoryAssertion) => void;
}) {
  if (query.isLoading) return <Padded><LoadingRows rows={6} /></Padded>;
  if (query.error) {
    return <Padded><ErrorState error={query.error} onRetry={() => void query.refetch()} /></Padded>;
  }
  if (page.visible.length === 0) return <Padded><ReaderEmpty kind="assertions" /></Padded>;
  return (
    <>
      <MemoryAssertionIndex
        assertions={page.visible}
        selectedID={selectedID}
        onSelect={onSelect}
      />
      <Padded>
        <LoadMore
          loaded={page.loaded}
          total={page.total}
          hasMore={page.hasMore}
          isLoading={false}
          onLoad={page.loadMore}
        />
      </Padded>
    </>
  );
}

function SuggestionList({
  query,
  page,
  selectedID,
  onSelect,
}: {
  query: UseQueryResult<{ items: MemorySuggestion[] }, Error>;
  page: ReturnType<typeof useVisibleItems<MemorySuggestion>>;
  selectedID?: string;
  onSelect: (suggestion: MemorySuggestion) => void;
}) {
  if (query.isLoading) return <Padded><LoadingRows rows={6} /></Padded>;
  if (query.error) {
    return <Padded><ErrorState error={query.error} onRetry={() => void query.refetch()} /></Padded>;
  }
  if (page.visible.length === 0) return <Padded><ReaderEmpty kind="suggestions" /></Padded>;
  return (
    <>
      <MemorySuggestionIndex
        suggestions={page.visible}
        selectedID={selectedID}
        onSelect={onSelect}
      />
      <Padded>
        <LoadMore
          loaded={page.loaded}
          total={page.total}
          hasMore={page.hasMore}
          isLoading={false}
          onLoad={page.loadMore}
        />
      </Padded>
    </>
  );
}

function ReaderEmpty({ kind }: { kind: "assertions" | "suggestions" }) {
  const { t } = useTranslation();
  const icon = kind === "suggestions" ? <Lightbulb className="size-6" /> : <Search className="size-6" />;
  return (
    <EmptyState
      icon={icon}
      title={t(kind === "suggestions" ? "memory.noSuggestions" : "memory.emptyTitle")}
      hint={t(kind === "suggestions" ? "memory.noSuggestionsHint" : "memory.emptyHint")}
    />
  );
}

function Padded({ children }: { children: ReactNode }) {
  return <div className="p-4">{children}</div>;
}
