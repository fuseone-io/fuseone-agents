import { useTranslation } from "react-i18next";
import { Lightbulb } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import type { Tool } from "@/features/admin/api";

/**
 * What the platform already believes this tool does, and why.
 *
 * A well-known server offers dozens of tools, and a Curator ruling on each from
 * a list of bare names is a Curator who stops reading by the tenth. The
 * suggestion exists so the act is a confirmation instead — which only works if
 * the reasoning is here, above the fields, before anything is filled in.
 *
 * It fills the form and never submits it. The classification stays the
 * Curator's act, recorded with their name and their reason: a suggestion that
 * applied itself would put the decision in a table shipped in a binary, which
 * is the same mistake as trusting the server about itself.
 */
export function SuggestedRuling({
  suggested,
  onAccept,
}: {
  suggested: NonNullable<Tool["suggested"]>;
  onAccept: () => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-2 rounded-lg border border-border bg-muted p-3">
      <p className="flex items-center gap-2 text-xs font-medium">
        <Lightbulb className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />
        {t("admin.platformSuggests")}{" "}
        <Mono className="text-2xs">{t(`effect.${suggested.effect}`)}</Mono>
      </p>

      <p className="text-2xs leading-relaxed text-muted-foreground">
        {suggested.why}
      </p>

      <Button
        type="button"
        variant="outline"
        size="sm"
        className="h-6 w-fit text-2xs"
        onClick={onAccept}
      >
        {t("admin.useTheSuggestion")}
      </Button>
    </div>
  );
}
