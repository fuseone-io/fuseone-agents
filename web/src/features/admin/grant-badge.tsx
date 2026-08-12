import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/shared/mono";
import type { HeldGrant } from "@/features/admin/people-api";

/**
 * One role, where it applies, and whether it can be taken away here.
 *
 * An asserted grant reads differently on purpose: it is re-derived on every
 * sign-in, so the thing to change is the group, and a screen that looked
 * editable would send somebody to the wrong place.
 */
export function GrantBadge({ grant }: { grant: HeldGrant }) {
  const { t } = useTranslation();

  return (
    <Badge
      variant={grant.asserted ? "outline" : "secondary"}
      className="gap-1.5 font-normal"
      title={grant.asserted ? t("people.assertedHint") : undefined}
    >
      <span>{t(`roles.${grant.role}`)}</span>
      <Mono className="text-2xs opacity-70">
        {grant.area === "" ? grant.company : `${grant.company}/${grant.area}`}
      </Mono>
      {grant.asserted && (
        <span className="text-2xs opacity-70">{t("people.asserted")}</span>
      )}
    </Badge>
  );
}
