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
  const sealed = entries.filter((entry) => entry.hash).length;
  const unsealed = entries.length - sealed;

  return (
    <section className="flex flex-wrap items-center gap-3 rounded-xl border border-border bg-success-surface px-4 py-3">
      <ShieldCheck className="size-4 shrink-0 text-success" aria-hidden />
      <p className="text-sm">
        <Mono>{sealed}</Mono> entradas do ledger, encadeadas por hash — cada uma
        sela a anterior.
      </p>

      {unsealed > 0 && (
        <p className="flex items-center gap-2 text-sm text-muted-foreground">
          <ShieldQuestion className="size-4 shrink-0" aria-hidden />
          <Mono>{unsealed}</Mono> da trilha administrativa: append-only por
          concessão, sem encadeamento.
        </p>
      )}

      <span className="ml-auto text-xs text-muted-foreground">
        Verificar a cadeia é por execução, na trilha dela.
      </span>
    </section>
  );
}
