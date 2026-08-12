import { Skeleton } from "@/components/ui/skeleton";
import { useStepContent } from "@/features/runs/api";

/**
 * What a step referenced, fetched when somebody opens it.
 *
 * Never with the trail: the trail is read constantly and by many people, and
 * arguments and results routinely carry personal data. Opening one step is a
 * deliberate act; loading all of them would not be.
 */
export function StepContent({
  runId,
  seq,
  open,
}: {
  runId: string;
  seq: number;
  open: boolean;
}) {
  const content = useStepContent(runId, seq, open);

  if (content.isLoading)
    return <Skeleton className="mt-2.5 h-16 w-full rounded-lg" />;
  if (content.error || !content.data) {
    return (
      <p className="mt-2.5 rounded-lg border border-border bg-muted p-3 text-xs text-muted-foreground">
        O conteúdo não está mais disponível. A retenção o apaga; a trilha guarda
        o resumo criptográfico dele.
      </p>
    );
  }

  return (
    <pre className="mt-2.5 overflow-x-auto rounded-lg border border-border bg-muted p-3 font-mono text-xs">
      {pretty(content.data.content)}
    </pre>
  );
}

function pretty(body: string): string {
  try {
    return JSON.stringify(JSON.parse(body), null, 2);
  } catch {
    return body;
  }
}
