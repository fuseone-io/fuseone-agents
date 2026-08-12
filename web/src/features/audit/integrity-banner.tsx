import { Trans, useTranslation } from "react-i18next";
import { ShieldCheck, ShieldQuestion } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import type { AuditEntry } from "@/lib/api/client";

/**
 * What can and cannot be proved about what is on screen.
 *
 * The handoff's banner reads "chain verified"; that is true of the ledger and
 * not of the administrative trail, which is append-only by grant — nobody
 * holds UPDATE — but is not hash-chained. Claiming one guarantee for both
 * would be the audit screen misreporting the thing it exists to report.
 */
export function IntegrityBanner({ entries }: { entries: AuditEntry[] }) {
  const { t } = useTranslation();
  const sealed = entries.filter((entry) => entry.hash).length;
  const unsealed = entries.length - sealed;

  return (
    <section className="flex flex-wrap items-center gap-3 rounded-xl border border-border bg-success-surface px-4 py-3">
      <ShieldCheck className="size-4 shrink-0 text-success" aria-hidden />
      <p className="text-sm">
        <Trans
          i18nKey="audit.sealedEntries"
          values={{ count: sealed }}
          components={{ n: <Mono /> }}
        />
      </p>

      {unsealed > 0 && (
        <p className="flex items-center gap-2 text-sm text-muted-foreground">
          <ShieldQuestion className="size-4 shrink-0" aria-hidden />
          <Trans
            i18nKey="audit.unsealedEntries"
            values={{ count: unsealed }}
            components={{ n: <Mono /> }}
          />
        </p>
      )}

      <span className="ml-auto text-xs text-muted-foreground">
        {t("audit.verifyIsPerRun")}
      </span>
    </section>
  );
}
