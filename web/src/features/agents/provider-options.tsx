import { useTranslation } from "react-i18next";
import { SelectItem } from "@/components/ui/select";
import type { ProviderChoice } from "@/features/agents/providers-api";

/**
 * The providers a picker offers, each marked when this installation would
 * not ask it now (`usable: false`).
 *
 * An unusable provider stays listed, so an agent already on it reads as such,
 * and is refused only as a new choice: the one already selected stays
 * selectable, or the picker would show the value as blank and an edit made
 * for another reason would look like it changed the provider.
 */
export function ProviderOptions({
  providers,
  current,
}: {
  providers: ProviderChoice[];
  /** What is selected now, which stays selectable whatever it is. */
  current?: string;
}) {
  const { t } = useTranslation();
  return providers.map((p) => (
    <SelectItem
      key={p.name}
      value={p.name}
      disabled={!p.usable && p.name !== current}
      className="font-mono"
    >
      {p.usable ? p.name : t("agents.providerUnavailable", { name: p.name })}
    </SelectItem>
  ));
}
