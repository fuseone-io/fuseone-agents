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
import { usePeople } from "@/features/admin/people-api";
import { Labelled, Section } from "@/features/policies/section";
import type { AgentDefinition } from "@/lib/api/client";

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
  const people = usePeople();
  const direct = draft.approvals?.direct ?? false;
  const notify = draft.approvals?.notify ?? [];
  const choosable = (people.data?.items ?? []).filter((one) => !one.disabled);

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
            disabled={people.isLoading}
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
                  {person.display || person.email || person.id}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            {t("agents.approvalsNotifyHint")}
          </p>
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
function displayOf(
  people: { id: string; display?: string | null; email?: string | null }[],
  id: string,
) {
  const found = people.find((one) => one.id === id);
  return found?.display || found?.email || id;
}
