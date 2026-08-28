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
    evidence: [{ runId: values.evidenceRunId.trim() }],
    reason: values.reason.trim(),
  };
}
