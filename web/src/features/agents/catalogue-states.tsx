import type { ReactNode } from "react";
import { LoadingRows } from "@/components/shared/states";
import { TargetError } from "@/features/agents/target-error";

export interface CatalogueState {
  isLoading: boolean;
  error: unknown;
  refetch: () => unknown;
}

/**
 * The catalogue tab's loading and error states, around the loaded one. The
 * empty state is the section's own, which knows what an empty catalogue
 * means.
 */
export function CatalogueStates({
  state,
  children,
}: {
  state: CatalogueState;
  children: ReactNode;
}) {
  if (state.isLoading) {
    return (
      <div className="p-5">
        <LoadingRows rows={6} />
      </div>
    );
  }
  if (state.error) {
    return (
      <div className="p-5">
        <TargetError error={state.error} onRetry={() => void state.refetch()} />
      </div>
    );
  }
  return <>{children}</>;
}
