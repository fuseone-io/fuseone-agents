import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/shared/mono";
import { EffectBadge } from "@/features/admin/effect-badge";
import { WithdrawCeiling } from "@/features/admin/withdraw-ceiling";
import type { Tool, ToolRuling } from "@/features/admin/api";

/**
 * The rulings on a tool as the listing reports them (NT-019): the platform's
 * first, which wins, and an owner's under it marked overridden rather than
 * hidden — somebody ruled, and the screen says why it is not applied.
 */
export function ToolRulings({
  tool,
  canWithdraw,
}: {
  tool: Tool;
  /** The platform's curator, who alone may withdraw the ceiling. */
  canWithdraw: boolean;
}) {
  const { t } = useTranslation();
  const rulings = tool.rulings ?? [];
  if (rulings.length === 0) return null;
  const ceiling = rulings.some((ruling) => ruling.level === "platform");

  return (
    <section className="flex flex-col items-start gap-2">
      <ul aria-label={t("admin.rulings")} className="w-full divide-y rounded-lg border">
        {rulings.map((ruling) => (
          <RulingRow key={ruling.level} ruling={ruling} />
        ))}
      </ul>
      {canWithdraw && ceiling && <WithdrawCeiling toolId={tool.toolId} />}
    </section>
  );
}

function RulingRow({ ruling }: { ruling: ToolRuling }) {
  const { t } = useTranslation();
  const where = ruling.scope.company === "*"
    ? t("admin.rulingInstallation")
    : [ruling.scope.company, ruling.scope.area].filter(Boolean).join("/");
  return (
    <li className="flex flex-wrap items-center gap-2 px-3 py-2 text-xs">
      <span className="font-medium">{t(`admin.rulingLevel.${ruling.level}`)}</span>
      <Mono className="text-muted-foreground">{where}</Mono>
      <EffectBadge effect={ruling.effect} />
      {ruling.inForce && <Badge variant="secondary">{t("admin.rulingInForce")}</Badge>}
      {ruling.overridden && (
        <Badge variant="outline">{t("admin.rulingOverridden")}</Badge>
      )}
    </li>
  );
}
