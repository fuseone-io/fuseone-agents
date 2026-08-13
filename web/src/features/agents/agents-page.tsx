import { useTranslation } from "react-i18next";
import { usePreferences, useTab } from "@/features/preferences/use-preferences";
import { useMemo, useState } from "react";
import { Bot } from "lucide-react";
import { Link } from "react-router-dom";
import { MessagesSquare, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { Toolbar } from "@/components/shared/toolbar";
import {
  FilterSelect,
  type FilterOption,
} from "@/components/shared/filter-select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Panel } from "@/components/shared/panel";
import { AgentCard } from "@/features/agents/agent-card";
import { AgentsTable } from "@/features/agents/agents-table";
import {
  AgentsViewToggle,
  type AgentsView,
} from "@/features/agents/agents-view";
import { EventGraph } from "@/features/agents/event-graph";
import { useAgents, type Agent } from "@/features/agents/api";
import { stateOfAgent, type AgentState } from "@/lib/agent-state";

export function AgentsPage() {
  const { t } = useTranslation();
  // Remembered rather than reset. Somebody who reads this screen in list form
  // reads it in list form tomorrow, and re-choosing on every visit is the kind
  // of small friction nobody reports and everybody feels.
  const history = useTab("agents", "latest");
  // Cards by default: most installations have a screenful of agents, and the
  // question at that size is how each one is doing rather than which is which.
  const view = usePreferences((s) => s.agentsView) as AgentsView;
  const setView = usePreferences((s) => s.setAgentsView);
  const [search, setSearch] = useState("");
  const [area, setArea] = useState("all");
  const [state, setState] = useState<AgentState | "all">("all");

  const { data, isLoading, error, refetch } = useAgents(
    history.value === "all",
  );
  const agents = useMemo(() => data?.items ?? [], [data]);

  // Filtered here rather than at the API, and that is defensible only because
  // this list is not paginated: what the browser holds is the whole answer.
  // The runs list is paginated, which is why its search is the server's.
  const areas = useMemo(
    () => [...new Set(agents.map((a) => a.scope.area))].sort(),
    [agents],
  );
  const shown = useMemo(
    () => agents.filter(matcher(search, area, state)),
    [agents, search, area, state],
  );

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.agents}
        title={t("nav.agents")}
        description={t("agents.subtitle")}
      >
        <Button size="sm" variant="outline" asChild>
          <Link to="/agents/interview">
            <MessagesSquare className="size-4" aria-hidden />
            {t("interview.title")}
          </Link>
        </Button>
        <Button size="sm" asChild>
          <Link to="/agents/new">
            <Plus className="size-4" aria-hidden />
            {t("agents.newAgent")}
          </Link>
        </Button>
      </PageHeader>

      {/* Everything that changes the view sits on one row, filters first and
          shape last: a reader narrows the set moving left to right, and how it
          is drawn is the last thing they decide. None of it belongs in the
          header — that is where the screen's one action lives. */}
      <Toolbar
        placeholder="agents.searchPlaceholder"
        value={search}
        onChange={setSearch}
        trailing={
          <>
            <Tabs {...history}>
              <TabsList>
                <TabsTrigger value="latest">{t("agents.current")}</TabsTrigger>
                <TabsTrigger value="all">{t("agents.history")}</TabsTrigger>
              </TabsList>
            </Tabs>
            <AgentsViewToggle view={view} onChange={setView} />
          </>
        }
      >
        <FilterSelect
          label={t("agents.filterByArea")}
          value={area}
          options={[
            { value: "all", label: t("agents.allAreas") },
            ...areas.map((a) => ({ value: a, label: a })),
          ]}
          onChange={setArea}
          width={170}
        />
        <FilterSelect
          label="agents.filterByState"
          value={state}
          options={STATES}
          onChange={(v) => setState(v as AgentState | "all")}
          width={190}
        />
      </Toolbar>

      {isLoading ? (
        <LoadingRows rows={3} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : shown.length === 0 ? (
        <EmptyState
          icon={<Bot className="size-6" />}
          title={
            agents.length === 0
              ? t("agents.nonePublished")
              : t("common.nothingFound")
          }
          hint={
            agents.length === 0 ? t("agents.emptyHint") : t("agents.noMatch")
          }
        />
      ) : view === "cards" ? (
        <div className="grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(268px,1fr))]">
          {shown.map((agent) => (
            <AgentCard
              key={`${agent.agentId}@${agent.versionId}`}
              agent={agent}
            />
          ))}
        </div>
      ) : (
        <Panel flush>
          <AgentsTable agents={shown} />
        </Panel>
      )}

      {/* Below the list, because the wiring is a fact about all of them and
          nobody comes to this screen looking for it first. It has to be here
          somewhere, though: composition is by event and neither side names the
          other, so this is the only place the graph is visible (PRD SE-10). */}
      <EventGraph />
    </>
  );
}

// An agent has no state of its own — the platform has no autonomy stage yet.
// These are the states of its runs, which is what an operator is actually
// asking about: what is going, what is stuck, what has never run.
const STATES: FilterOption[] = [
  { value: "all", label: "agents.allStates" },
  { value: "running", label: "runs.phaseRunning" },
  { value: "waiting", label: "runs.waitingPerson" },
  { value: "blocked", label: "runs.phaseParked" },
  { value: "done", label: "agents.lastFinished" },
  { value: "draft", label: "agents.neverRanShort" },
];

function matcher(search: string, area: string, state: AgentState | "all") {
  const q = search.trim().toLowerCase();
  return (agent: Agent) => {
    if (area !== "all" && agent.scope.area !== area) return false;
    if (state !== "all" && stateOfAgent(agent.activity?.lastPhase) !== state)
      return false;
    if (!q) return true;
    return (
      agent.name.toLowerCase().includes(q) ||
      agent.agentId.toLowerCase().includes(q)
    );
  };
}
