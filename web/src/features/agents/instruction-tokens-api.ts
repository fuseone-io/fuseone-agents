import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { useSettled } from "@/hooks/use-settled";

/** How long the text has to stand still before it is worth asking about. */
const SETTLES_AFTER_MS = 700;

/**
 * How large the instruction is, asked of the model that will read it.
 *
 * The console cannot answer this. Tokenisation belongs to the model — it
 * differs between vendors and between generations of the same vendor — so a
 * count computed here would be wrong for a model released after the release,
 * in the one place somebody goes to size a prompt. The server asks the
 * provider the agent is configured to use.
 *
 * The answer is kept for as long as the screen is open: the same text, model
 * and provider always count to the same number, so a text somebody scrolls
 * back to is not asked about twice.
 *
 * A provider with no counting endpoint answers that it has none, which is a
 * state rather than a failure — the card then shows characters and says
 * characters.
 */
export function useInstructionTokens(
  provider: string,
  model: string,
  instructions: string,
) {
  const settled = useSettled(instructions, SETTLES_AFTER_MS);
  const asked = settled.trim();

  const { data } = useQuery({
    queryKey: ["instruction-tokens", provider, model, asked],
    enabled: Boolean(provider && model && asked),
    staleTime: Infinity,
    // One attempt. A provider that is unreachable stays unreachable for the
    // next keystroke too, and three tries per edit would turn a configuration
    // problem into load.
    retry: false,
    queryFn: async () =>
      unwrap(
        await api.POST("/agents/instructions/tokens", {
          body: { provider, model, instructions: asked },
        }),
      ),
  });

  // Undefined means nothing counted it, which is the only honest answer the
  // card can render as anything other than tokens.
  return data?.counted ? data.tokens : undefined;
}
