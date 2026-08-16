import { useTranslation } from "react-i18next";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Mono } from "@/components/shared/mono";
import { EffectBadge } from "@/features/admin/effect-badge";
import type { Tool } from "@/features/admin/api";
import { remoteNameOf } from "@/features/integrations/mcp/tool-names";

/**
 * Of what this server offers, which tools this installation brought in.
 *
 * The box is not a permission. Outside the surface a tool is not "allowed with
 * conditions" — it is not here: no model is told about it, no call reaches it,
 * and the Gate is never asked. What the box decides is scope, which is what
 * keeps a server with two hundred tools from becoming two hundred rulings.
 *
 * The effect beside it is the Curator's separate act, shown because the two
 * questions are asked in the same breath and answered by different people:
 * bringing a tool in is not saying what it does.
 */
export function SurfacePicker({
  tools,
  chosen,
  onToggle,
}: {
  tools: Tool[];
  chosen: Set<string>;
  onToggle: (remoteName: string, next: boolean) => void;
}) {
  const { t } = useTranslation();

  if (tools.length === 0) {
    return <p className="text-xs text-muted-foreground">{t("mcp.noToolsYet")}</p>;
  }

  return (
    <ul className="divide-y rounded-xl border">
      {tools.map((tool) => (
        <ToolRow
          key={tool.toolId}
          tool={tool}
          chosen={chosen.has(remoteNameOf(tool))}
          onToggle={onToggle}
        />
      ))}
    </ul>
  );
}

function ToolRow({
  tool,
  chosen,
  onToggle,
}: {
  tool: Tool;
  chosen: boolean;
  onToggle: (remoteName: string, next: boolean) => void;
}) {
  const { t } = useTranslation();
  const declaredBy = tool.declaredBy ?? [];
  const leaving = !chosen && declaredBy.length > 0;

  return (
    <li className="flex items-start gap-3 p-3">
      <Checkbox
        id={tool.toolId}
        checked={chosen}
        onCheckedChange={(next) => onToggle(remoteNameOf(tool), next === true)}
        className="mt-0.5"
      />
      <div className="min-w-0 flex-1">
        <Label htmlFor={tool.toolId} className="cursor-pointer">
          <Mono>{tool.toolId}</Mono>
        </Label>
        {tool.description && (
          <p className="truncate text-xs text-muted-foreground">
            {tool.description}
          </p>
        )}
        {/* Said where the choice is made, not discovered in production. Off
            the surface the tool is not a capability this installation has, and
            an agent that still names it stops at the Gate. */}
        {leaving && (
          <p className="mt-1 text-xs text-danger">
            {t("mcp.leavingAffects", {
              count: declaredBy.length,
              agents: declaredBy.join(", "),
            })}
          </p>
        )}
      </div>
      <EffectBadge effect={tool.effect} stale={tool.stale} />
    </li>
  );
}
