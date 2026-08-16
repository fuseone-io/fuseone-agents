import { Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  TRIGGER_KINDS,
  emptyTrigger,
  fieldOf,
  incomplete,
  type TriggerKind,
} from "@/features/agents/trigger-kinds";
import type { AgentTrigger } from "@/lib/api/client";

/** One trigger: its kind, the one thing that kind needs, and a way out. */
export function TriggerRow({
  trigger,
  onChange,
  onRemove,
}: {
  trigger: AgentTrigger;
  onChange: (over: Partial<AgentTrigger>) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const kind = trigger.type;
  const field = fieldOf(kind);

  return (
    <div className="flex items-start gap-2">
      <Select
        value={kind}
        onValueChange={(next) =>
          // The value of the old kind is dropped rather than carried: a cron
          // expression left in a webhook's path is a trigger that publishes
          // and never fires.
          onChange(emptyTrigger(next as TriggerKind))
        }
      >
        <SelectTrigger className="w-40 shrink-0">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {TRIGGER_KINDS.map((option) => (
            <SelectItem key={option} value={option}>
              {t(`agents.trigger.${option}`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {/* A channel trigger has no field: the author declares that an ask in a
          conversation of this agent's scope may start it, and which
          conversations belong to which scope is administrative. An input here
          would be the author choosing who may start their own agent. */}
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        {field === undefined ? (
          <p className="text-xs text-muted-foreground">
            {t("agents.triggerChannelExplains")}
          </p>
        ) : (
          <>
            <Input
              className="font-mono"
              aria-label={t(`agents.triggerField.${kind}`)}
              placeholder={t(`agents.triggerExample.${kind}`)}
              value={String(trigger[field] ?? "")}
              onChange={(e) => onChange({ [field]: e.target.value })}
            />
            {incomplete(trigger) && (
              <p className="text-2xs text-warning">
                {t(`agents.triggerNeeds.${kind}`)}
              </p>
            )}
          </>
        )}
      </div>

      <Button
        type="button"
        variant="ghost"
        size="icon"
        aria-label={t("common.remove")}
        onClick={onRemove}
      >
        <Trash2 className="size-4" />
      </Button>
    </div>
  );
}
