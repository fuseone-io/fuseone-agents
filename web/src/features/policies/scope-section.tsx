import { Input } from "@/components/ui/input";
import { Section, Labelled } from "@/features/policies/section";
import { cn } from "@/lib/utils";
import type { PolicyInput } from "@/lib/api/client";

const COVERED = ["read", "write", "destructive", "financial"] as const;

/** Scope: what the rule covers before any condition is read. */
export function ScopeSection({
  draft,
  patch,
}: {
  draft: PolicyInput;
  patch: (over: Partial<PolicyInput>) => void;
}) {
  const covered = draft.effects ?? [];

  return (
    <Section title="Escopo" hint="O que a regra alcança antes de qualquer condição.">
      <Labelled label="Ferramenta" htmlFor="resource">
        <Input
          id="resource"
          value={draft.resource ?? ""}
          onChange={(e) => patch({ resource: e.target.value })}
          placeholder="crm.* ou crm.reply ou *"
          className="font-mono"
        />
      </Labelled>

      <fieldset className="flex flex-col gap-1.5">
        <legend className="text-2xs uppercase tracking-label text-muted-foreground">
          Efeitos cobertos
        </legend>
        <div className="flex flex-wrap gap-1.5">
          {COVERED.map((effect) => {
            const on = covered.includes(effect);
            return (
              <button
                key={effect}
                type="button"
                aria-pressed={on}
                onClick={() =>
                  patch({
                    effects: on ? covered.filter((e) => e !== effect) : [...covered, effect],
                  })
                }
                className={cn(
                  "h-7 rounded-pill border px-2.5 font-mono text-2xs",
                  on ? "border-primary bg-surface-accent text-text-accent" : "border-border",
                )}
              >
                {effect}
              </button>
            );
          })}
        </div>
        <p className="text-xs text-muted-foreground">
          Nenhum selecionado cobre todos.
        </p>
      </fieldset>
    </Section>
  );
}
