import { Lock, ShieldX, UserRoundCheck, type LucideIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Checkbox } from "@/components/ui/checkbox";
import { Mono } from "@/components/shared/mono";
import { EffectBadge, UntrustedBadge } from "@/features/agents/effect-badge";
import { ruleFor } from "@/features/agents/tool-rule";
import type { ToolGroup } from "@/features/agents/tool-catalogue";
import { cn } from "@/lib/utils";
import type { Policy } from "@/lib/api/client";

const RULE_TONE = {
  allowed: "text-muted-foreground",
  asks: "text-warning",
  blocked: "text-danger",
};

/** The handoff's glyphs: a person for the queue, a shield for a refusal. */
const RULE_ICON: Record<string, LucideIcon | undefined> = {
  allowed: undefined,
  asks: UserRoundCheck,
  blocked: ShieldX,
};

/** One server's tools, under a heading that says how many are already granted. */
export function ToolGroupRows({
  group,
  granted,
  policies,
  onToggle,
}: {
  group: ToolGroup;
  granted: string[];
  policies: Policy[];
  onToggle: (toolId: string) => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col">
      <div className="flex items-baseline gap-2 border-b border-border py-1.5">
        <Mono className="text-xs font-medium">{group.server}</Mono>
        <span className="text-2xs tabular-nums text-muted-foreground">
          {t("agents.grantedOf", {
            granted: group.granted,
            total: group.tools.length,
          })}
        </span>
      </div>

      <ul className="flex flex-col">
        {group.tools.map((tool) => {
          const rule = ruleFor(tool.toolId, tool.effect, policies);
          return (
            <li
              key={tool.toolId}
              className={cn(
                "grid grid-cols-[24px_1fr_120px_1fr] items-center gap-3 border-b border-border-subtle py-2 last:border-0",
                // Visible but unavailable, never hidden: an author has to be
                // able to see that a policy denies something rather than
                // wonder where it went.
                rule.kind === "blocked" && "opacity-55",
              )}
            >
              {/* A lock where a policy denies it. The checkbox would offer a
                  choice the platform will not honour, and a tool ticked here
                  that the Gate refuses is an agent whose first attempt at that
                  call fails. */}
              {rule.kind === "blocked" ? (
                <span
                  className="flex size-4 items-center justify-center text-muted-foreground"
                  title={t("agents.deniedByPolicy")}
                >
                  <Lock
                    className="size-3.5"
                    aria-label={t("agents.deniedByPolicy")}
                  />
                </span>
              ) : (
                <Checkbox
                  checked={granted.includes(tool.toolId)}
                  onCheckedChange={() => onToggle(tool.toolId)}
                  aria-label={t("agents.grantTool", { tool: tool.toolId })}
                />
              )}

              <div className="min-w-0">
                <span className="flex items-center gap-1.5">
                  <Mono className="truncate">{tool.toolId}</Mono>
                  {tool.untrusted && <UntrustedBadge />}
                </span>
                {tool.description && (
                  <span className="block truncate text-2xs text-muted-foreground">
                    {tool.description}
                  </span>
                )}
              </div>

              <EffectBadge effect={tool.effect} />

              <div className="flex min-w-0 items-center justify-end gap-1.5">
                {rule.because && (
                  <Mono dim className="truncate text-2xs">
                    {rule.because}
                  </Mono>
                )}
                {/* An icon beside the word, never instead of it: the three
                    outcomes have to be told apart without relying on colour. */}
                <RuleGlyph kind={rule.kind} />
                <span className={cn("truncate text-xs", RULE_TONE[rule.kind])}>
                  {t(rule.label, rule.labelValues)}
                </span>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

/** The glyph for one verdict, or nothing where the answer is simply yes. */
function RuleGlyph({ kind }: { kind: keyof typeof RULE_TONE }) {
  const Glyph = RULE_ICON[kind];
  if (!Glyph) return null;
  return (
    <Glyph className={cn("size-3.5 shrink-0", RULE_TONE[kind])} aria-hidden />
  );
}
