import type { ReactNode } from "react";
import type { TFunction } from "i18next";
import { JsonBody } from "@/components/shared/json-body";
import { Mono } from "@/components/shared/mono";
import type { DraftOperation } from "@/lib/api/client";

type FieldKind = Exclude<DraftOperation["kind"], "insert_block" | "replace_block" | "edit_block" | "remove_block" | "move_block">;

const json = (value: unknown) => <JsonBody body={JSON.stringify(value, null, 2)} />;
const line = (text: string) => <Mono className="text-sm">{text}</Mono>;

/**
 * What a field operation sets, whole: the value it writes, or, for a field it
 * clears, what clearing it means. Undefined when the operation does not carry
 * what its kind needs, so the card can refuse to offer it: nobody accepts what
 * they were not shown. Every kind is named, so a new one is a compile error
 * here rather than a card that shows nothing.
 */
export function fieldValue(op: DraftOperation, t: TFunction): ReactNode | undefined {
  const kind = op.kind as FieldKind;
  switch (kind) {
    case "add_tool":
    case "remove_tool":
      return op.tool ? line(op.tool) : undefined;
    case "set_model":
      return op.model ? line([op.provider, op.model, op.effort].filter(Boolean).join(" · ")) : undefined;
    case "set_fallback":
      return line(op.fallback ? `${op.fallback.provider} · ${op.fallback.model}` : t("agents.fallbackNone"));
    case "set_cache_prefix_ttl":
      return line(op.cachePrefixTtl || t("agents.cacheTtlDefault"));
    case "set_reply_locale":
      return line(op.replyLocale || t("agents.replyLocaleDefault"));
    case "set_budget":
      return op.budget ? json(op.budget) : undefined;
    case "set_triggers":
      return json(op.triggers ?? []);
    case "set_memory_learning":
      return op.memoryLearning ? json(op.memoryLearning) : undefined;
    case "set_approval_delivery":
      return op.approvals ? json(op.approvals) : undefined;
    case "set_emits":
      return json(op.emits ?? []);
    case "add_step":
      return op.step ? json(op.step) : undefined;
    case "edit_step":
      return op.stepName && op.step ? (
        <div className="flex flex-col gap-1">
          <p className="text-sm">{t("workbench.conversation.value.editsStep", { step: op.stepName })}</p>
          {json(op.step)}
        </div>
      ) : undefined;
    case "remove_step":
      return op.stepName ? line(op.stepName) : undefined;
    case "set_identity":
      return json({ name: op.name, company: op.company, area: op.area });
    case "set_standing_approvals":
      return op.standingApprovals ? json(op.standingApprovals) : undefined;
    default: {
      const unnamed: never = kind;
      return unnamed;
    }
  }
}
