import { Mono } from "@/components/shared/mono";
import { draftSentence } from "@/features/policies/policy-sentence";
import type { Change } from "@/features/policies/policy-form";
import type { PolicyInput } from "@/lib/api/client";

/**
 * What saving this will mean.
 *
 * The handoff puts a simulation here — the draft run against the last 500
 * runs, with the count of would-be denials. That needs replay, which does not
 * exist yet, so this rail says what it can say honestly and states the gap
 * rather than showing a reassuring number nobody computed.
 */
export function PolicySideRail({
  draft,
  creating,
  changes,
}: {
  draft: PolicyInput;
  creating: boolean;
  changes: Change[];
}) {
  return (
    <aside className="flex flex-col gap-3 lg:sticky lg:top-0">
      <Card title="A regra">
        <Mono className="block break-words text-xs">{draftSentence(draft)}</Mono>
        <p className="text-xs text-muted-foreground">
          {draft.mode === "monitor"
            ? "Vai ser avaliada em todo passo que o escopo cobre, registrada na trilha, e não vai mudar nenhuma decisão."
            : "Vai valer no próximo passo de agente depois que os workers recarregarem o conjunto."}
        </p>
      </Card>

      {creating ? (
        <Card title="Ordem de avaliação">
          <p className="text-xs text-muted-foreground">
            A ordem entre políticas não muda o resultado: o Portão devolve a
            decisão mais restritiva entre todas que casam. Negar vence escalar,
            que vence permitir.
          </p>
          <p className="text-xs text-muted-foreground">
            A única exceção é um <span className="text-foreground">permitir</span> que
            casou: é a única coisa que afrouxa o padrão embutido, e é o que
            uma exceção é.
          </p>
        </Card>
      ) : (
        <Card title={`Sem gravar (${changes.length})`}>
          {changes.length === 0 ? (
            <p className="text-xs text-muted-foreground">Nada mudou ainda.</p>
          ) : (
            <ul className="flex flex-col gap-1.5">
              {changes.map((change) => (
                <li key={change.field} className="text-xs">
                  <span className="text-warning">~</span> {change.field}{" "}
                  <Mono dim className="text-2xs">
                    {change.from} → {change.to}
                  </Mono>
                </li>
              ))}
            </ul>
          )}
        </Card>
      )}

      <Card title="Simulação">
        {/* Said plainly. The alternative is a panel that looks like it ran
            something, which on a screen about governance is worse than one
            that admits it did not. */}
        <p className="text-xs text-muted-foreground">
          Ainda não é possível rodar esta regra contra execuções passadas antes
          de gravá-la. Até lá, o caminho seguro é gravar em modo monitorar e
          ler na trilha o que ela teria feito.
        </p>
      </Card>
    </aside>
  );
}

function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="flex flex-col gap-2 rounded-xl border border-border bg-card p-4 shadow-sm">
      <h2 className="text-2xs uppercase tracking-label text-muted-foreground">{title}</h2>
      {children}
    </section>
  );
}
