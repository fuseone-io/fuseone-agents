import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { useSendTurn } from "@/features/workbench/conversation-api";
import { problemMessage } from "@/lib/api/problem-message";

type Draft = { text: string; pasted: boolean; key?: string };

/**
 * The message being written, and the key it is sent under.
 *
 * The key is chosen the first time a message is sent and kept until it is
 * answered: a send that never got an answer, sent again, names the same turn,
 * so the model is never called twice for it. Changing the message is another
 * turn, with another key. Pasting marks the message, which informs the
 * proposal's card and fences nothing.
 */
export function useComposer(agentId: string) {
  const { t, i18n } = useTranslation();
  const sending = useSendTurn(agentId);
  const [draft, setDraft] = useState<Draft>({ text: "", pasted: false });

  const write = (text: string) =>
    setDraft((was) => ({ text, pasted: text === "" ? false : was.pasted, key: undefined }));
  const paste = () => setDraft((was) => ({ ...was, pasted: true }));

  const send = () => {
    if (draft.text.trim() === "") return;
    const key = draft.key ?? crypto.randomUUID();
    setDraft((was) => ({ ...was, key }));
    sending.mutate(
      { key, message: draft.text, pasted: draft.pasted, locale: i18n.language },
      {
        onSuccess: () => setDraft({ text: "", pasted: false }),
        onError: (error) => toast.error(problemMessage(error, t)),
      },
    );
  };

  return {
    text: draft.text,
    pasted: draft.pasted,
    write,
    paste,
    send,
    sending: sending.isPending,
    pending: sending.isPending ? sending.variables?.message : undefined,
  };
}

export type Composer = ReturnType<typeof useComposer>;
