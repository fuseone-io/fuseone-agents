import { FileText } from "lucide-react";
import { Mono } from "@/components/shared/mono";

/**
 * What somebody told the agent to do, exactly as published.
 *
 * Read-only, and it will stay read-only: a specification is changed by
 * publishing a new version, never by editing one that runs already reference.
 * An editable box here would let somebody rewrite the explanation of a run
 * that already happened.
 */
export function AgentDefinition({
  instructions,
  source,
}: {
  instructions?: string;
  source?: string;
}) {
  return (
    <section className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-sm">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="text-sm font-medium">Definição</h2>
        {source && <Mono dim className="truncate">{source}</Mono>}
        <span className="ml-auto text-xs text-muted-foreground">
          publicada, não editável
        </span>
      </div>

      {instructions ? (
        <p className="whitespace-pre-wrap text-sm leading-relaxed">{instructions}</p>
      ) : (
        <p className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
          <FileText className="size-4" aria-hidden />
          Esta versão foi publicada sem instruções.
        </p>
      )}
    </section>
  );
}
