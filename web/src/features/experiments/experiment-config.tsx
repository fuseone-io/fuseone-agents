import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import type { ExperimentConfig } from "@/features/experiments/experiments-api";

/**
 * What an experiment overrode, field by field.
 *
 * Only what it overrode: an absent field is the version's own, and naming the
 * version's value here would read as a choice somebody made. An experiment that
 * overrode nothing says so in words, because it is the version as published —
 * the one configuration that keeps the declared fallback.
 */
export function ExperimentConfigText({ config }: { config: ExperimentConfig }) {
  const { t } = useTranslation();
  const parts = [config.provider, config.model, config.effort].filter(
    (part): part is string => Boolean(part),
  );

  if (parts.length === 0) {
    return <span>{t("experiments.asPublished")}</span>;
  }
  return <Mono className="text-xs">{parts.join(" · ")}</Mono>;
}
