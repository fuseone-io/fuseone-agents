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
    evidenceArtifact: "final_answer",
    evidenceDigest: "",
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
    evidence: [
      {
        runId: values.evidenceRunId.trim(),
        artifact: values.evidenceArtifact.trim(),
        digest: values.evidenceDigest.trim(),
      },
    ],
    reason: values.reason.trim(),
  };
}
