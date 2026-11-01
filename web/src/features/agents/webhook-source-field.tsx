import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { eventsFrom, needsEvents } from "@/features/agents/trigger-kinds";
import type { AgentTrigger } from "@/lib/api/client";

const ANY = "any";

/**
 * Who sends a webhook's deliveries, and for GitHub which of them start a run.
 *
 * Any sender puts the secret in a header and names each delivery. GitHub signs
 * with the secret instead and sends every event the hook subscribes to, so a
 * GitHub webhook also names its events: `event` for every action of it,
 * `event.action` for one.
 *
 * The events are kept as typed text while editing, so a comma typed before the
 * next name is not swallowed; what reaches the trigger is the parsed list. The
 * text follows the trigger whenever the two disagree: the rows are keyed by
 * position, so removing one hands this component to the next row, and text
 * left from the removed row would be shown over — and written into — the
 * remaining one.
 */
export function WebhookSourceField({
  trigger,
  onChange,
}: {
  trigger: AgentTrigger;
  onChange: (over: Partial<AgentTrigger>) => void;
}) {
  const { t } = useTranslation();
  const declared = trigger.events ?? [];
  const [text, setText] = useState(declared.join(", "));
  if (eventsFrom(text).join(",") !== declared.join(",")) {
    // Adjusting state while rendering, as React documents for state that must
    // follow a prop: the trigger changed under this text.
    setText(declared.join(", "));
  }
  const github = trigger.from === "github";

  return (
    <div className="flex flex-col gap-1">
      <Select
        value={github ? "github" : ANY}
        onValueChange={(next) => {
          // Back to any sender, the events go: they would be read by nobody.
          setText("");
          onChange(
            next === "github"
              ? { from: "github" }
              : { from: undefined, events: undefined },
          );
        }}
      >
        <SelectTrigger aria-label={t("agents.webhookSource.label")}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ANY}>{t("agents.webhookSource.any")}</SelectItem>
          <SelectItem value="github">
            {t("agents.webhookSource.github")}
          </SelectItem>
        </SelectContent>
      </Select>
      {github && (
        <>
          <Input
            className="font-mono"
            aria-label={t("agents.webhookSource.events")}
            placeholder={t("agents.webhookSource.eventsExample")}
            value={text}
            onChange={(e) => {
              setText(e.target.value);
              onChange({ events: eventsFrom(e.target.value) });
            }}
          />
          {needsEvents(trigger) && (
            <p className="text-2xs text-warning">
              {t("agents.webhookSource.needsEvents")}
            </p>
          )}
          {/* Publishing checks only the shape of a name, so a misspelt one
              publishes and never fires: say so, and where to look. */}
          <p className="text-2xs text-muted-foreground">
            {t("agents.webhookSource.eventsHint")}
          </p>
        </>
      )}
    </div>
  );
}
