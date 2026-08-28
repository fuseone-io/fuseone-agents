import { useWatch, type UseFormReturn } from "react-hook-form";
import { useMemoryMatch } from "@/features/memory/api";
import type { MemoryFormValues } from "@/features/memory/memory-form-schema";
import { useRun } from "@/features/runs/api";
import { useSettled } from "@/hooks/use-settled";

type MemoryCreateMatchIssue =
  | { kind: "run"; retry: () => void }
  | { kind: "agent" }
  | { kind: "match"; retry: () => void };

/**
 * Resolves the evidence run before asking what the platform already knows.
 *
 * Creation derives its agent from the ledger, so the preview must do the same.
 * Letting the form provide an agent would make the warning and the write ask
 * about different identities. The run id comes from an explicit picker, so it
 * can be inspected immediately rather than debounced like free text.
 */
export function useMemoryCreateMatch(
  form: UseFormReturn<MemoryFormValues>,
  options: { enabled?: boolean } = {},
) {
  const { enabled = true } = options;
  const values = useWatch({ control: form.control });
  const runID = values.evidenceRunId?.trim() ?? "";
  const run = useRun(runID, enabled && Boolean(runID));
  const currentIdentity = JSON.stringify([
    values.company?.trim() ?? "",
    values.area?.trim() ?? "",
    values.namespace ?? "agent",
    values.kind?.trim() ?? "",
    values.subject?.trim() ?? "",
    values.signature?.trim() ?? "",
  ]);
  const settledIdentity = useSettled(currentIdentity, 400);
  const identityInputSettled = currentIdentity === settledIdentity;
  const [company, area, rawNamespace, kind, subject, signature] = JSON.parse(
    settledIdentity,
  ) as string[];
  const namespace = rawNamespace === "shared" ? "shared" : "agent";
  const identityComplete = Boolean(
    values.company?.trim() &&
      values.area?.trim() &&
      values.kind?.trim() &&
      values.subject?.trim() &&
      values.signature?.trim() &&
      runID,
  );
  const runReady = run.isSuccess;
  const agentMissing =
    runReady && namespace === "agent" && !run.data.agentId;
  const match = useMemoryMatch(
    {
      company: company ?? "",
      area: area ?? "",
      namespace,
      ...(namespace === "agent" && run.data?.agentId
        ? { agentId: run.data.agentId }
        : {}),
      kind: kind ?? "",
      subject: subject ?? "",
      signature: signature ?? "",
    },
    { enabled: enabled && runReady && identityInputSettled, settle: false },
  );
  const matchSettled = match.isSuccess || match.isError;
  const ready =
    !enabled ||
    !identityComplete ||
    (runReady && identityInputSettled && !agentMissing && matchSettled);
  let issue: MemoryCreateMatchIssue | null = null;
  if (enabled && run.error) {
    issue = { kind: "run", retry: () => void run.refetch() };
  } else if (enabled && agentMissing) {
    issue = { kind: "agent" };
  } else if (enabled && identityInputSettled && match.error) {
    // Match is advisory. The write still merges identities and rejects conflicts.
    issue = { kind: "match", retry: () => void match.refetch() };
  }

  return {
    data: match.isSuccess ? match.data : undefined,
    required: identityComplete,
    ready,
    loading: identityComplete && !ready && !issue,
    issue,
  };
}
