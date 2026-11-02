import type { TFunction } from "i18next";
import { toast } from "sonner";
import { eventCycleMessage } from "@/features/agents/event-cycle-warning";
import type { components } from "@/lib/api/schema.gen";
import type { Locale } from "@/i18n/locale";

type AgentPublished = components["schemas"]["AgentPublished"];

/**
 * What the author is told once a version is published.
 *
 * The warning is a second toast rather than a longer first one: the version
 * was written either way, and that fact should read the same whether or not
 * something about it deserves attention. It stays until dismissed, because
 * the screen moves to the agent's page as it appears.
 */
export function announcePublished(
  result: AgentPublished,
  t: TFunction,
  locale: Locale,
): void {
  toast.success(
    result.created
      ? t("agents.versionPublished", { version: result.versionId.slice(0, 9) })
      : t("agents.noChange"),
  );
  const warning = eventCycleMessage(result.eventCycleWarning, t, locale);
  if (warning) toast.warning(warning, { duration: Infinity });
}
