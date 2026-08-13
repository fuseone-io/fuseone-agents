import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { useSaveChannel } from "@/features/channels/api";
import { problemMessage } from "@/lib/api/problem-message";
import type { components } from "@/lib/api/schema.gen";

const schema = z.object({
  name: z.string().min(1, "channels.needsName"),
  workspace: z.string(),
  token: z.string(),
});

/**
 * Connecting a workspace.
 *
 * The credential is write-only, in both directions: the form never receives
 * one and an empty field on an edit keeps what is stored. Correcting a
 * workspace's name must not demand re-entering a secret nobody has to hand.
 */
export function ChannelForm({
  channel,
  onClose,
}: {
  channel: components["schemas"]["Channel"] | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const save = useSaveChannel();

  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: channel?.name ?? "",
      workspace: channel?.workspace ?? "",
      token: "",
    },
  });

  async function submit(values: z.infer<typeof schema>) {
    try {
      await save.mutateAsync({
        name: values.name.trim(),
        kind: "slack",
        workspace: values.workspace.trim() || undefined,
        token: values.token.trim() || undefined,
      });
      toast.success(t("channels.saved"));
      onClose();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {channel ? t("channels.editChannel") : t("channels.newChannel")}
          </DialogTitle>
          <DialogDescription>{t("channels.explains")}</DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(submit)}
            className="flex flex-col gap-4"
          >
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("admin.name")}</FormLabel>
                  <FormControl>
                    <Input {...field} disabled={channel !== null} />
                  </FormControl>
                  <FormDescription>
                    {t("channels.nameExplains")}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="workspace"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("channels.workspace")}</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder={t("common.optional")} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="token"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("channels.botToken")}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type="password"
                      autoComplete="off"
                      placeholder={
                        channel?.hasCredential
                          ? t("channels.tokenStored")
                          : "xoxb-…"
                      }
                    />
                  </FormControl>
                  <FormDescription>{t("channels.tokenSealed")}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={save.isPending}>
                {t("common.save")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
