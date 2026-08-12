import { useTranslation } from "react-i18next";
import { Checkbox } from "@/components/ui/checkbox";
import { Mono } from "@/components/shared/mono";
import { Section } from "@/features/policies/section";
import { ruleFor } from "@/features/agents/tool-rule";
import { cn } from "@/lib/utils";
import type { Policy, Tool } from "@/lib/api/client";

const RULE_TONE = {
  allowed: "text-success",
  asks: "text-warning",
  blocked: "text-danger",
};

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
  const toggle = (tool: string) =>
    onChange(
      granted.includes(tool)
        ? granted.filter((t) => t !== tool)
        : [...granted, tool],
    );

  return (
    <Section
      title="Ferramentas"
      hint="O que não está aqui não pode ser invocado, o que quer que as instruções peçam."
    >
      {catalogue.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t("agents.emptyCatalogue")}
        </p>
      ) : (
        <ul className="flex flex-col">
          {catalogue.map((tool) => {
            const on = granted.includes(tool.toolId);
            const rule = ruleFor(tool.toolId, tool.effect, policies);

            return (
              <li
                key={tool.toolId}
                className={cn(
                  "grid grid-cols-[24px_1fr_96px_1fr] items-center gap-3 border-b border-border-subtle py-2 last:border-0",
                  // Visible but unavailable, never hidden: an author has to be
                  // able to see that a policy denies something rather than
                  // wonder where it went.
                  rule.kind === "blocked" && "opacity-55",
                )}
              >
                <Checkbox
                  checked={on}
                  onCheckedChange={() => toggle(tool.toolId)}
                  aria-label={`Conceder ${tool.toolId}`}
                />

                <div className="min-w-0">
                  <Mono className="block truncate">{tool.toolId}</Mono>
                  <span className="block truncate text-2xs text-muted-foreground">
                    {tool.server}
                    {tool.description ? ` · ${tool.description}` : ""}
                  </span>
                </div>

                <span className="font-mono text-2xs text-muted-foreground">
                  {tool.effect}
                </span>

                <div className="min-w-0 text-right">
                  <span className={cn("text-xs", RULE_TONE[rule.kind])}>
                    {rule.label}
                  </span>
                  {rule.because && (
                    <Mono dim className="ml-1.5 text-2xs">
                      {rule.because}
                    </Mono>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      )}

      <p className="text-xs text-muted-foreground">
        {t("agents.rightColumnIsPolicy")}
      </p>
    </Section>
  );
}
