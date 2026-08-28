import { useWatch, type UseFormReturn } from "react-hook-form";
import { useMemoryMatch } from "@/features/memory/api";
import type { MemoryFormValues } from "@/features/memory/memory-form-schema";
import { useRun } from "@/features/runs/api";
import { useSettled } from "@/hooks/use-settled";

/**
 * Resolves the evidence run before asking what the platform already knows.
 *
 * Creation derives its agent from the ledger, so the preview must do the same.
 * Letting the form provide an agent would make the warning and the write ask
 * about different identities. The run id settles first to avoid one 404 for
 * every prefix while somebody types or pastes it.
 */
export function useMemoryCreateMatch(form: UseFormReturn<MemoryFormValues>) {
  const values = useWatch({ control: form.control });
  const rawRunID = values.evidenceRunId?.trim() ?? "";
  const runID = useSettled(rawRunID, 400);
  const runInputSettled = rawRunID === runID;
  const run = useRun(runID, runInputSettled && Boolean(runID));
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
      rawRunID,
  );
  const runReady = runInputSettled && run.isSuccess;
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
    { enabled: runReady && identityInputSettled, settle: false },
  );
  const ready =
    !identityComplete ||
    (runReady && identityInputSettled && match.isSuccess);
  const error = runInputSettled
    ? run.error ??
      (agentMissing ? new Error("memory: evidence run has no agent") : null) ??
      (identityInputSettled ? match.error : null)
    : null;

  return {
    data: ready ? match.data : undefined,
    required: identityComplete,
    ready,
    loading: identityComplete && !ready && !error,
    error,
    retry: () =>
      void (run.error || agentMissing ? run.refetch() : match.refetch()),
  };
}
