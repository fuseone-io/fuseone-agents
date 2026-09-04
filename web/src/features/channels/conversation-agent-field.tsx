import type { UseFormReturn } from "react-hook-form";
import { useTranslation } from "react-i18next";
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAgentsForScope } from "@/features/channels/api";
import type { ConversationValues } from "@/features/channels/conversation-form";
import type { components } from "@/lib/api/schema.gen";

// A shadcn Select cannot carry an empty value, and "nobody chose one" is a real
// choice here rather than an unset field. So it travels as a sentinel and is
// turned back into an empty string on the way out.
const NO_AGENT = "__none__";

/**
 * Which agent this conversation is for.
 *
 * One field for both start modes, because it is one decision. A watched message
 * has always needed it — there is no text to read a name out of — and a mention
 * needs it to be able to arrive without naming an agent, which is the whole
 * point of pointing a channel at one.
 *
 * Optional only where the platform can still fall back to the message naming
 * one, so "none" is offered for mentions and never for watched messages: a
 * watched conversation with no agent starts nothing at all.
 */
export function ConversationAgentField({
  form,
  mode,
  scope,
}: {
  form: UseFormReturn<ConversationValues>;
  mode: "mentions" | "watch" | "both";
  scope: string;
}) {
  const { t } = useTranslation();
  const agents = useAgentsForScope(scope);
  const startable = (agents.data?.items ?? []).filter(startableFromConversation);
  const optional = mode === "mentions";

  return (
    <FormField
      control={form.control}
      name="agent"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t("channels.conversationAgent")}</FormLabel>
          <Select
            onValueChange={(picked) =>
              field.onChange(picked === NO_AGENT ? "" : picked)
            }
            value={field.value || (optional ? NO_AGENT : "")}
            disabled={!scope || agents.isLoading}
          >
            <FormControl>
              <SelectTrigger>
                <SelectValue placeholder={t("channels.pickWatchAgent")} />
              </SelectTrigger>
            </FormControl>
            <SelectContent>
              {optional && (
                <SelectItem value={NO_AGENT}>
                  {t("channels.anyAgentNamed")}
                </SelectItem>
              )}
              {startable.map((agent) => (
                <SelectItem key={agent.agentId} value={agent.agentId}>
                  {agent.name || agent.agentId}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <FormDescription>
            {agents.isSuccess && startable.length === 0
              ? t("channels.noWatchAgents")
              : mode === "watch"
                ? t("channels.watchAgentExplains")
                : t("channels.conversationAgentExplains")}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

function startableFromConversation(agent: components["schemas"]["Agent"]) {
  return (agent.triggers ?? []).some((trigger) => trigger.type === "channel");
}
