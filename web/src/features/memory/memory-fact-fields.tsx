import { useWatch, type Control } from "react-hook-form";
import { MemoryDerivedIdentity } from "@/features/memory/memory-derived-identity";
import {
  MemoryInputField,
  MemoryTextareaField,
} from "@/features/memory/memory-form-fields";
import type { MemoryFormValues } from "@/features/memory/memory-form-schema";

/**
 * What the memory says: what it is about and the claim itself. The internal
 * identity is shown, but it is derived rather than authored.
 *
 * Shared by the two ways of teaching one, because this half is the same in
 * both. What surrounds it is not, and is deliberately not shared: on the memory
 * page a person names the run they learned from, and in a run's own sheet the
 * run is given and the artifact is picked from what the ledger recorded. One
 * form with a flag for which screen it is would put both governed shapes in one
 * place and let a rule meant for one of them reach the other.
 */
export function MemoryFactFields({
  control,
}: {
  control: Control<MemoryFormValues>;
}) {
  const subject = useWatch({ control, name: "subject" });
  return (
    <>
      <MemoryInputField
        control={control}
        name="subject"
        label="memory.subject"
        placeholder="memory.subjectPlaceholder"
        description="memory.subjectIdentityHint"
      />
      <MemoryDerivedIdentity subject={subject} />
      <MemoryTextareaField
        control={control}
        name="claim"
        label="memory.claim"
        description="memory.claimHint"
      />
    </>
  );
}
