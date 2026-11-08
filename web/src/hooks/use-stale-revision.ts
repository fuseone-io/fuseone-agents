import { createElement, useState, type ReactNode } from "react";
import { StaleNotice } from "@/components/shared/stale-notice";
import { isStale, type FormRevision } from "@/lib/api/revision";

/**
 * What a form needs to answer a save refused as stale: whether this error was
 * that refusal, and the notice to show in its place. Nothing for a form with
 * no revision, which is one creating something.
 */
export function useStaleRevision(revision?: FormRevision): {
  notice: ReactNode;
  refused: (error: unknown) => boolean;
} {
  const [stale, setStale] = useState(false);
  return {
    notice:
      stale && revision
        ? createElement(StaleNotice, {
            onReload: revision.reload,
            reloading: revision.reloading,
          })
        : null,
    refused: (error) => {
      if (!revision || !isStale(error)) return false;
      setStale(true);
      return true;
    },
  };
}
