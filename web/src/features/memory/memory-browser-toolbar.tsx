import { AlertTriangle, Search } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { MemoryFilters } from "@/features/memory/api";
import type { MemoryView } from "@/features/memory/memory-view";

const MAX_MEMORY_SEARCH_TERMS = 6;
const MEMORY_SEARCH_ENGLISH_STOPWORDS = new Set([
  "about",
  "anything",
  "are",
  "could",
  "find",
  "for",
  "from",
  "know",
  "need",
  "please",
  "problem",
  "search",
  "some",
  "something",
  "tell",
  "that",
  "the",
  "these",
  "thing",
  "this",
  "those",
  "want",
  "were",
  "with",
  "without",
]);
const MEMORY_SEARCH_PORTUGUESE_STOPWORD =
  /^(?:algum|alguma|algumas|alguns|aquela|aquele|aqueles|aquilo|aqui|com|coisa|coisas|como|consegue|da|das|de|delas|deles|do|dos|e|em|essa|essas|esse|esses|esta|estas|este|estes|eu|favor|isso|isto|me|na|nas|no|nos|o|os|para|pela|pelas|pelo|pelos|por|preciso|problema|procure|procurar|quais|qual|qualquer|que|queria|quero|saber|sem|sobre|um|uma|umas|uns|voc(?:e|\u00ea))$/u;

export function MemoryBrowserToolbar({
  filters,
  view,
  onFilters,
  onView,
}: {
  filters: MemoryFilters;
  view: MemoryView;
  onFilters: (filters: MemoryFilters) => void;
  onView: (view: MemoryView) => void;
}) {
  const { t } = useTranslation();
  const budget = memorySearchTermBudget(filters.search);
  return (
    <div className="flex flex-wrap items-start gap-3 border-b px-4 py-3">
      <div className="relative min-w-56 flex-1">
        <Search
          className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
          aria-hidden
        />
        <Input
          type="search"
          aria-label={t("memory.search")}
          placeholder={t("memory.searchPlaceholder")}
          value={filters.search}
          onChange={(event) => onFilters({ ...filters, search: event.target.value })}
          className="h-8 pl-8"
        />
      </div>
      <Input
        aria-label={t("memory.agentFilter")}
        placeholder={t("memory.agentFilterPlaceholder")}
        value={filters.agentId}
        onChange={(event) => onFilters({ ...filters, agentId: event.target.value })}
        className="h-8 min-w-44 flex-1 font-mono sm:max-w-56"
      />
      <Tabs value={view} onValueChange={(next) => onView(next as MemoryView)}>
        <TabsList className="h-8" aria-label={t("memory.viewFilter")}>
          <TabsTrigger value="active">{t("memory.status.active")}</TabsTrigger>
          <TabsTrigger value="disabled">{t("memory.status.disabled")}</TabsTrigger>
          <TabsTrigger value="suggested">{t("memory.suggestions")}</TabsTrigger>
          <TabsTrigger value="all">{t("memory.status.all")}</TabsTrigger>
        </TabsList>
      </Tabs>
      {budget.omitted > 0 ? (
        <p
          role="status"
          className="flex basis-full items-start gap-1.5 text-xs text-warning"
        >
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" aria-hidden />
          <span>{t("memory.searchTermBudgetNotice", { count: MAX_MEMORY_SEARCH_TERMS })}</span>
        </p>
      ) : null}
    </div>
  );
}

function memorySearchTermBudget(search: string): { omitted: number } {
  const seen = new Set<string>();
  const strong: string[] = [];
  const ordinary: string[] = [];
  for (const raw of search.trim().toLowerCase().split(/\s+/)) {
    if (!raw) continue;
    const trimmed = raw.replace(/^["'.,;:()[\]{}<>!?]+|["'.,;:()[\]{}<>!?]+$/g, "");
    const term = (trimmed || raw.trim()).slice(0, 64);
    if (!term || seen.has(term)) continue;
    seen.add(term);
    if (strongSearchTerm(term)) {
      strong.push(term);
    } else if (!memorySearchStopword(term)) {
      ordinary.push(term);
    }
  }
  const omitted = Math.max(0, strong.length + ordinary.length - MAX_MEMORY_SEARCH_TERMS);
  return { omitted };
}

function strongSearchTerm(term: string): boolean {
  if (term.length < 6) return false;
  if (/[_.\-/:=]/.test(term)) return true;
  return /[a-z]/i.test(term) && /\d/.test(term);
}

function memorySearchStopword(term: string): boolean {
  if ([...term].length < 2) return true;
  return (
    MEMORY_SEARCH_ENGLISH_STOPWORDS.has(term) ||
    MEMORY_SEARCH_PORTUGUESE_STOPWORD.test(term)
  );
}
