import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";

const VERDICT: Record<
  string,
  { label: string; variant: "secondary" | "destructive" | "outline" }
> = {
  allow: { label: "simulation.verdictAllow", variant: "secondary" },
  constrain: { label: "simulation.verdictConstrain", variant: "outline" },
  require_approval: {
    label: "simulation.verdictApproval",
    variant: "outline",
  },
  block: { label: "simulation.verdictBlock", variant: "destructive" },
};

/** The Gate's ruling, as a word beside its colour — a chip that read only red
 *  would carry its meaning in a channel a third of readers cannot use. */
export function VerdictBadge({ verdict }: { verdict: string }) {
  const { t } = useTranslation();
  const shown = VERDICT[verdict];
  if (!shown) return <Badge variant="outline">{verdict}</Badge>;
  return <Badge variant={shown.variant}>{t(shown.label)}</Badge>;
}
