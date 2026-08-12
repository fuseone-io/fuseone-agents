import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import { useSimulatePolicy } from "@/features/policies/api";
import { cn } from "@/lib/utils";
import type { PolicyInput, Simulation } from "@/lib/api/client";

/**
 * What this rule would have done to runs that already happened.
 *
 * Run on demand rather than as you type: it reads the trail, and a panel that
 * re-queried on every keystroke would make writing a careful rule expensive in
 * proportion to how carefully it was written.
 */
export function SimulationCard({ draft }: { draft: PolicyInput }) {
  const simulate = useSimulatePolicy();
  const result = simulate.data;

  return (
    <section className="flex flex-col gap-2.5 rounded-xl border border-border bg-card p-4 shadow-sm">
      <h2 className="text-2xs uppercase tracking-label text-muted-foreground">
        Simulação
      </h2>

      {!result ? (
        <p className="text-xs text-muted-foreground">
          Roda esta regra contra as decisões já registradas e mostra o que ela
          teria feito, antes de gravar.
        </p>
      ) : (
        <Result result={result} />
      )}

      <Button
        variant="outline"
        size="sm"
        className="h-8"
        disabled={simulate.isPending}
        onClick={() => simulate.mutate(draft)}
      >
        {simulate.isPending
          ? "Rodando…"
          : result
            ? "Rodar de novo"
            : "Rodar contra o histórico"}
      </Button>
    </section>
  );
}

function Result({ result }: { result: Simulation }) {
  // Zero out of zero is not evidence a rule is harmless, and this is the line
  // that keeps a quiet installation from reading as a safe rule.
  if (result.considered === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        Nenhuma decisão registrada no período, então não há o que simular. Isso
        não diz que a regra é inofensiva — diz que não houve com o que comparar.
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-2.5">
      <div className="grid grid-cols-2 gap-2">
        <Tile label="Bateriam" value={result.matched} />
        <Tile
          label="Seriam negadas"
          value={result.wouldDeny ?? 0}
          alarming={(result.wouldDeny ?? 0) > 0}
        />
      </div>

      <p className="text-xs text-muted-foreground">
        Sobre <Mono dim>{result.considered}</Mono> decisões registradas.
      </p>

      {/* The number that keeps this panel honest. A rule reading arguments
          cannot be replayed against a call that never stored any, and
          reporting those as no-match would show zero denials for exactly the
          rule somebody was nervous about. */}
      {result.unknown > 0 && (
        <p className="text-xs text-warning">
          <Mono>{result.unknown}</Mono> decisões não puderam ser respondidas:
          esta regra lê argumentos, e chamadas recusadas nunca guardaram nenhum.
          O número acima é um piso, não um total.
        </p>
      )}

      {result.samples.length > 0 && (
        <ul className="flex flex-col gap-1">
          {result.samples.map((sample) => (
            <li key={`${sample.runId}-${sample.seq}`} className="text-2xs">
              <Link to={`/runs/${sample.runId}`} className="hover:underline">
                <Mono dim>{sample.tool}</Mono>{" "}
                <span className="text-muted-foreground">
                  {sample.was} → {sample.wouldBe}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function Tile({
  label,
  value,
  alarming,
}: {
  label: string;
  value: number;
  alarming?: boolean;
}) {
  return (
    <div
      className={cn(
        "rounded-lg border p-2.5",
        alarming ? "border-danger bg-danger-surface" : "border-border bg-muted",
      )}
    >
      <div className="text-2xs text-muted-foreground">{label}</div>
      <div
        className={cn(
          "font-mono text-lg font-medium tabular-nums",
          alarming && "text-danger",
        )}
      >
        {value}
      </div>
    </div>
  );
}
