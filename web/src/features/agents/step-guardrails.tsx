import { useTranslation } from "react-i18next";
import { CircleCheck, CircleHelp, OctagonX } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import { ruleFor } from "@/features/agents/tool-rule";
import type { Policy, Tool } from "@/lib/api/client";

/**
 * What the Gate will do with what this stage reaches.
 *
 * Read, never set. The answer comes from the effect ladder and whatever policy
 * covers the call, and offering it here as a switch would be a fourth place
 * deciding whether a person is asked — beside the ladder, the policies and the
 * taint rules. Four sources of truth about one question is how an operator
 * ends up unable to say why something was blocked.
 *
 * It belongs on the stage rather than only on the pack because the pack is the
 * ceiling and the stage is the permission: an author looking at one card wants
 * to know what happens *here*, and "somewhere in this agent there is a
 * financial tool" is not that.
 */
export function StepGuardrails({
  reaches,
  catalogue,
  policies,
}: {
  reaches: string[];
  catalogue: Tool[];
  policies: Policy[];
}) {
  const { t } = useTranslation();

  if (reaches.length === 0) {
    return (
      <p className="text-2xs text-muted-foreground">
        {t("agents.nothingToGate")}
      </p>
    );
  }

  return (
    <ul className="flex flex-col gap-2">
      {reaches.map((tool) => {
        const effect =
          catalogue.find((one) => one.toolId === tool)?.effect ?? "write";
        const rule = ruleFor(tool, effect, policies);
        const Icon = ICONS[rule.kind];

        return (
          <li key={tool} className="flex flex-col gap-0.5">
            <Mono className="text-2xs">{tool}</Mono>
            <span className={`flex items-center gap-1.5 text-2xs ${TONE[rule.kind]}`}>
              <Icon className="size-3.5 shrink-0" aria-hidden />
              {t(rule.label, rule.labelValues)}
            </span>
            {/* The rule that produced it, when a policy did rather than the
                ladder: "blocked by policy" tells an author nothing about what
                to change, and cannot tell two rules apart. */}
            {rule.because && (
              <Mono dim className="text-2xs">
                {rule.because}
              </Mono>
            )}
          </li>
        );
      })}
    </ul>
  );
}

const ICONS = {
  allowed: CircleCheck,
  asks: CircleHelp,
  blocked: OctagonX,
};

const TONE = {
  allowed: "text-success",
  asks: "text-warning",
  blocked: "text-danger",
};
