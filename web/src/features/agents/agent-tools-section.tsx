import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Section } from "@/features/policies/section";
import { ToolGroupRows } from "@/features/agents/tool-group-rows";
import { grouped, matching } from "@/features/agents/tool-catalogue";
import type { Policy, Tool } from "@/lib/api/client";

/**
 * What this agent may call, and what will happen when it does.
 *
 * The right-hand column is derived from the policies in force rather than set
 * here. Making it a per-agent setting would be a fourth place that decides
 * whether a human is asked, beside the ladder, the policies and the taint
 * check — and four answers to one question is how somebody ends up unable to
 * say why a call was blocked.
 */
export function AgentToolsSection({
  granted,
  catalogue,
  policies,
  onChange,
}: {
  granted: string[];
  catalogue: Tool[];
  policies: Policy[];
  onChange: (tools: string[]) => void;
}) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const toggle = (tool: string) =>
    onChange(
      granted.includes(tool)
        ? granted.filter((t) => t !== tool)
        : [...granted, tool],
    );

  const groups = grouped(matching(catalogue, query), granted);

  return (
    <Section title={t("admin.tools")} hint={t("agents.notHereNotInvokable")}>
      {catalogue.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t("agents.emptyCatalogue")}
        </p>
      ) : (
        <>
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("agents.searchTools")}
            aria-label={t("agents.searchTools")}
          />

          {groups.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              {t("agents.noToolMatches")}
            </p>
          ) : (
            groups.map((group) => (
              <ToolGroupRows
                key={group.server}
                group={group}
                granted={granted}
                policies={policies}
                onToggle={toggle}
              />
            ))
          )}
        </>
      )}

      <p className="text-xs text-muted-foreground">
        {t("agents.rightColumnIsPolicy")}
      </p>
    </Section>
  );
}
