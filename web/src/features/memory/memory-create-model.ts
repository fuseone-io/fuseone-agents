import type { MemoryAssertionInput } from "@/features/memory/api";
import type { MemoryFormValues } from "@/features/memory/memory-form-schema";

export function memoryDefaults(
  company: string,
  area: string,
): MemoryFormValues {
  return {
    company: company === "*" ? "" : company,
    area,
    namespace: "agent",
    kind: "",
    subject: "",
    signature: "",
    claim: "",
    evidenceRunId: "",
    evidenceArtifact: "",
    reason: "",
  };
}

export function toMemoryAssertionInput(
  values: MemoryFormValues,
): MemoryAssertionInput {
  return {
    company: values.company.trim(),
    area: values.area.trim(),
    namespace: values.namespace,
    kind: values.kind.trim(),
    subject: values.subject.trim(),
    signature: values.signature.trim(),
    claim: values.claim.trim(),
    // The run, and nothing else. Which of its outputs defaults to the closing
    // answer, and the digest is the ledger's to say — asking a person to copy
    // sixty-four characters into a form was asking them to do by hand what the
    // server does anyway.
    evidence: [
      {
        runId: values.evidenceRunId.trim(),
        ...(values.evidenceArtifact
          ? { artifact: values.evidenceArtifact }
          : {}),
      },
    ],
    reason: values.reason.trim(),
  };
}

/**
 * The form a run's sheet starts from.
 *
 * Scope, run and artifact are given rather than defaulted: they come from the
 * run being read and the step being cited, and none of them is a preference.
 * What stays empty is what only a person can say — what kind of fact this is,
 * what it is about, what it claims, and why they are recording it.
 *
 * The namespace starts at `agent`, the narrower reach. Shared memory is what
 * every agent in the scope recalls, and defaulting to it would make the widest
 * reach in the platform the one nobody chose.
 */
export function memoryFromRun(
  scope: { company: string; area: string },
  runId: string,
  artifact: string,
): MemoryFormValues {
  return {
    company: scope.company,
    area: scope.area,
    namespace: "agent",
    kind: "",
    subject: "",
    signature: "",
    claim: "",
    evidenceRunId: runId,
    evidenceArtifact: artifact,
    reason: "",
  };
}
