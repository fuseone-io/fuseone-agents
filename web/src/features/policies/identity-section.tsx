import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Section, Labelled } from "@/features/policies/section";
import type { PolicyInput } from "@/lib/api/client";

/** Identity: what the rule is called, and the sentence somebody denied reads. */
export function IdentitySection({
  draft,
  patch,
  code,
  editable,
  onCode,
}: {
  draft: PolicyInput;
  patch: (over: Partial<PolicyInput>) => void;
  code: string;
  editable: boolean;
  onCode: (code: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Section title={t("policies.identity")} hint={t("policies.codeAppears")}>
      <div className="grid gap-3 sm:grid-cols-[140px_1fr_200px]">
        <Labelled label={t("policies.code")} htmlFor="code">
          {/* Set once. It is in the trail and in support conversations, and a
              code that moved would orphan every one of them. */}
          <Input
            id="code"
            value={code}
            onChange={(e) => onCode(e.target.value)}
            disabled={!editable}
            readOnly={!editable}
            className="font-mono"
          />
        </Labelled>
        <Labelled label={t("admin.name")} htmlFor="name">
          <Input
            id="name"
            value={draft.name}
            onChange={(e) => patch({ name: e.target.value })}
          />
        </Labelled>
        <Labelled label={t("policies.owner")} htmlFor="owner">
          <Input
            id="owner"
            value={draft.owner ?? ""}
            onChange={(e) => patch({ owner: e.target.value })}
          />
        </Labelled>
      </div>

      <Labelled label={t("policies.reason")} htmlFor="reason">
        <Textarea
          id="reason"
          rows={2}
          value={draft.reason ?? ""}
          onChange={(e) => patch({ reason: e.target.value })}
          placeholder={t("policies.reasonPlaceholder")}
        />
        <p className="text-xs text-muted-foreground">
          {t("policies.reasonExplains", { code })}
        </p>
      </Labelled>
    </Section>
  );
}
