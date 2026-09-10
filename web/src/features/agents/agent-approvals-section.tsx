import { ShieldCheck } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useEligibleApprovers } from "@/features/agents/api";
import { Labelled, Section } from "@/features/policies/section";
import type { AgentDefinition } from "@/lib/api/client";
import { problemMessage } from "@/lib/api/problem-message";
import { cn } from "@/lib/utils";

/*
The most people one approval may be sent to privately.

The same number the fan-out stops at, and said here rather than only there: past
it nobody is messaged at all, because telling an arbitrary twenty of a hundred
is worse than telling none. A control that let somebody choose twenty-one would
be collecting a decision the platform refuses minutes later, on another screen.
*/
const MAX_NOTIFY = 20;

/**
 * Where the people who may decide are asked.
 *
 * The Gate decides *whether* a person must answer, and nothing here changes
 * that: the console always has the approval, and this only adds a private
 * message from the channel bot. So the section says what it does and never
 * offers to switch approval off — that would be a different decision, taken by
 * somebody else, in another screen.
 *
 * Naming people narrows who is told among those who may already decide. It
 * grants nothing: the button is checked against the run's own scope wherever it
 * is pressed, so a name here that holds no grant is dropped rather than sent a
 * message it cannot answer.
 */
export function AgentApprovalsSection({
  draft,
  patch,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  const { t } = useTranslation();
  const direct = draft.approvals?.direct ?? false;
  const notify = draft.approvals?.notify ?? [];
  // Only when a message was asked for, and only the people who may already
  // decide in this agent's own scope — the same list the fan-out will use, so
  // naming somebody here cannot produce a message that never goes out.
  const people = useEligibleApprovers(draft.company, draft.area, direct);
  const choosable = people.data?.items ?? [];
  const full = notify.length >= MAX_NOTIFY;

  // Absent rather than `{direct: false}`: an agent that asked for nothing
  // carries no block at all, so its file does not grow a section its author
  // never typed.
  const setDirect = (on: boolean) =>
    patch({ approvals: on ? { direct: true, notify } : undefined });
  const setNotify = (who: string[]) =>
    patch({ approvals: { direct: true, notify: who } });

  return (
    <Section
      icon={ShieldCheck}
      title={t("agents.approvals")}
      hint={t("agents.approvalsHint")}
    >
      <label className="flex items-start gap-2 text-sm">
        <Checkbox
          checked={direct}
          onCheckedChange={(on) => setDirect(Boolean(on))}
        />
        <span className="grid gap-1">
          <span className="font-medium">{t("agents.approvalsDirect")}</span>
          <span className="text-muted-foreground">
            {t("agents.approvalsDirectHint")}
          </span>
        </span>
      </label>

      {direct && (
        <Labelled
          label={t("agents.approvalsNotify")}
          htmlFor="approvals-notify"
        >
          <Select
            value=""
            disabled={people.isLoading || people.isError || full}
            onValueChange={(id) =>
              setNotify(notify.includes(id) ? notify : [...notify, id])
            }
          >
            <SelectTrigger id="approvals-notify">
              <SelectValue placeholder={t("agents.approvalsNotifyAnyone")} />
            </SelectTrigger>
            <SelectContent>
              {choosable.map((person) => (
                <SelectItem key={person.id} value={person.id}>
                  {person.display || person.id}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {/* The ceiling is said before somebody meets it: the twenty-first
              name is refused when they publish, which is minutes later and on
              another screen. */}
          <p
            className={cn(
              "text-xs",
              people.isError ? "text-warning" : "text-muted-foreground",
            )}
          >
            {people.isError
              ? problemMessage(people.error, t)
              : full
                ? t("agents.approvalsNotifyFull", { count: MAX_NOTIFY })
                : t("agents.approvalsNotifyHint")}
          </p>
          {people.isError && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="w-fit"
              disabled={people.isFetching}
              onClick={() => void people.refetch()}
            >
              {t("common.retry")}
            </Button>
          )}
          {notify.length > 0 && (
            <div className="flex flex-wrap gap-1">
              {notify.map((id) => (
                <Badge key={id} variant="outline" className="gap-1">
                  {displayOf(choosable, id)}
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="size-4"
                    aria-label={t("agents.approvalsNotifyRemove", {
                      person: displayOf(choosable, id),
                    })}
                    onClick={() =>
                      setNotify(notify.filter((one) => one !== id))
                    }
                  >
                    ×
                  </Button>
                </Badge>
              ))}
            </div>
          )}
        </Labelled>
      )}
    </Section>
  );
}

// The name if this console knows it, and the identifier if it does not: a
// person removed from the directory is still named in a published version, and
// showing nothing there would make the list look shorter than it is.
function displayOf(people: { id: string; display: string }[], id: string) {
  return people.find((one) => one.id === id)?.display || id;
}
