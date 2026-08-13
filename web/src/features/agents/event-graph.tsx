import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { ArrowRight, CircleAlert } from "lucide-react";
import { Panel } from "@/components/shared/panel";
import { EmptyState } from "@/components/shared/states";
import { Mono } from "@/components/shared/mono";
import { api, unwrap } from "@/lib/api/client";

/**
 * Who triggers whom (PRD SE-10).
 *
 * Composition is by event: one agent publishes, another consumes, and neither
 * names the other. That is what keeps it composition rather than a phone call,
 * and it is also why this screen has to exist — the wiring is in no single
 * definition, so without a picture of it nobody can review it.
 *
 * The dangling edges are the point. An event nobody listens to and a trigger
 * nothing publishes are the two mistakes, and a graph that hid them would look
 * correct while being wrong.
 */
export function EventGraph() {
  const { t } = useTranslation();
  const graph = useQuery({
    queryKey: ["event-graph"],
    queryFn: async () => unwrap(await api.GET("/agents/events")).edges,
  });

  const edges = graph.data ?? [];

  return (
    <Panel title={t("agents.eventGraph")} flush>
      {edges.length === 0 ? (
        <div className="p-4">
          <EmptyState
            title={t("agents.noEvents")}
            hint={t("agents.noEventsHint")}
          />
        </div>
      ) : (
        <ul className="flex flex-col gap-1.5 p-4 pt-0">
          {edges.map((edge) => (
            <li
              key={`${edge.from ?? ""}-${edge.event}-${edge.to ?? ""}`}
              className="flex flex-wrap items-center gap-2 rounded-lg border px-3 py-2 text-xs"
            >
              <Side agent={edge.from} missing={t("agents.nobodyPublishes")} />
              <ArrowRight aria-hidden className="size-3 text-muted-foreground" />
              <Mono className="text-xs">{edge.event}</Mono>
              <ArrowRight aria-hidden className="size-3 text-muted-foreground" />
              <Side agent={edge.to} missing={t("agents.nobodyListens")} />
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}

/** One end of an edge, or the reason there is nothing there. */
function Side({ agent, missing }: { agent?: string; missing: string }) {
  if (agent) return <Mono className="text-xs">{agent}</Mono>;
  return (
    <span className="flex items-center gap-1 text-warning">
      <CircleAlert aria-hidden className="size-3.5" />
      {missing}
    </span>
  );
}
