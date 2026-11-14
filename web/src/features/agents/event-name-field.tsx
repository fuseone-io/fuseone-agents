import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  SuggestPopover,
  type SuggestOption,
} from "@/components/shared/suggest-popover";
import { Input } from "@/components/ui/input";
import { useEventGraph, type EventEdge } from "@/features/agents/api";

/**
 * An event trigger's name, offered from the event graph.
 *
 * A typo publishes cleanly and never fires, so the names agents already
 * publish or consume are suggested beside the input. The input stays free:
 * an event nothing publishes yet is a legitimate trigger, and the graph is
 * where its dangling edge then shows. Asked for when the list opens.
 */
export function EventNameField({
  value,
  onChange,
}: {
  value: string;
  onChange: (event: string) => void;
}) {
  const { t } = useTranslation();
  const [asked, setAsked] = useState(false);
  const graph = useEventGraph(asked);
  return (
    <div className="flex min-w-0 items-center gap-1">
      <Input
        className="min-w-0 flex-1 font-mono"
        aria-label={t("agents.triggerField.event")}
        placeholder={t("agents.triggerExample.event")}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      <SuggestPopover
        label={t("agents.suggestEvent")}
        options={eventOptions(graph.data?.edges ?? [])}
        selected={value}
        onPick={onChange}
        onOpen={() => setAsked(true)}
      />
    </div>
  );
}

/** Each event once, hinted with an agent that publishes it when one does. */
function eventOptions(edges: EventEdge[]): SuggestOption[] {
  const byName = new Map<string, string | undefined>();
  for (const edge of edges) {
    if (edge.event === "") continue;
    byName.set(edge.event, byName.get(edge.event) ?? edge.from);
  }
  return [...byName]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([value, hint]) => ({ value, hint }));
}
