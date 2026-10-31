import { UserCheck } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Section } from "@/features/policies/section";
import type { AgentDefinition } from "@/lib/api/client";

/**
 * Whether a standing approval may ever answer for this agent.
 *
 * On, every approval the Gate asks of it waits for a person: no grant can be
 * created for it, and one made earlier is never used. That is what a pilot
 * measuring how often each role is right needs — a grant answering for the
 * role would count as a person's approval nobody gave.
 *
 * Off is the absence of the setting rather than "allowed", so the file carries
 * no line its author never typed and its digest stays the version it was.
 */
export function AgentStandingToggle({
  draft,
  patch,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  const { t } = useTranslation();
  const forbidden = draft.standingApprovals === "forbidden";

  return (
    <Section
      icon={UserCheck}
      title={t("agents.standingTitle")}
      hint={t("agents.standingHint")}
    >
      <div className="flex items-start gap-2 text-sm">
        <Switch
          id="standing-forbidden"
          aria-describedby="standing-forbidden-hint"
          checked={forbidden}
          onCheckedChange={(on) =>
            patch({ standingApprovals: on ? "forbidden" : undefined })
          }
        />
        <div className="grid gap-1">
          <Label htmlFor="standing-forbidden">
            {t("agents.standingForbid")}
          </Label>
          <p id="standing-forbidden-hint" className="text-muted-foreground">
            {t("agents.standingForbidHint")}
          </p>
        </div>
      </div>
    </Section>
  );
}
