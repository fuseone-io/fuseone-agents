import type { ReactNode } from "react";
import { AlertCircle, Inbox, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ApiError } from "@/lib/api/client";

// Every view that loads data owes the reader four states. These are the three
// that are easy to skip, so they live here and are cheap to reach for.

export function LoadingRows({ rows = 5 }: { rows?: number }) {
  return (
    <div className="flex flex-col gap-2" aria-busy="true" aria-live="polite">
      <span className="sr-only">Carregando</span>
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-12 w-full" />
      ))}
    </div>
  );
}

export function EmptyState({
  title,
  hint,
  icon,
}: {
  title: string;
  hint: string;
  icon?: ReactNode;
}) {
  return (
    <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed px-6 py-12 text-center">
      <div className="text-muted-foreground">{icon ?? <Inbox className="size-6" />}</div>
      <p className="font-medium">{title}</p>
      {/* An empty state that only says "nothing here" wastes the moment: say
          what would appear and what produces it. */}
      <p className="max-w-md text-sm text-muted-foreground">{hint}</p>
    </div>
  );
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const problem = error instanceof ApiError ? error : undefined;
  return (
    <div
      role="alert"
      className="flex flex-col items-start gap-3 rounded-lg border border-destructive/40 bg-destructive/5 px-6 py-5"
    >
      <div className="flex items-center gap-2 font-medium text-destructive">
        <AlertCircle className="size-4" />
        {problem?.message ?? "Não foi possível carregar"}
      </div>
      <p className="text-sm text-muted-foreground">
        {problem?.detail ?? "Verifique a conexão com o servidor e tente novamente."}
      </p>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry}>
          <RefreshCw className="size-4" />
          Tentar de novo
        </Button>
      )}
    </div>
  );
}
