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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useSaveChannel } from "@/features/channels/api";
import { problemMessage } from "@/lib/api/problem-message";
import type { components } from "@/lib/api/schema.gen";

const schema = z.object({
  name: z.string().min(1, "channels.needsName"),
  kind: z.string().min(1, "channels.needsKind"),
  workspace: z.string(),
  deliveryMode: z.enum(["http", "socket"]),
  token: z.string(),
  appToken: z.string(),
  signingSecret: z.string(),
});

/**
 * What the credential is called, per vendor.
 *
 * The kind used to be invisible: the only thing on the form saying this was
 * Slack was an `xoxb-…` placeholder. Naming it makes the choice explicit, and
 * it has to be a choice because there will be more than one.
 */
const CREDENTIAL: Record<string, { label: string; hint: string }> = {
  slack: { label: "channels.botToken", hint: "xoxb-…" },
};

/**
 * Connecting a workspace.
 *
 * The credentials are write-only, in both directions: the form never receives
 * them and an empty field on an edit keeps what belongs to the selected
 * delivery mode. Correcting a workspace's name must not demand re-entering a
 * secret nobody has to hand.
 */
export function ChannelForm({
  channel,
  kinds,
  onClose,
}: {
  channel: components["schemas"]["Channel"] | null;
  /** What this installation can connect, answered by the process. */
  kinds: string[];
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const save = useSaveChannel();

  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: channel?.name ?? "",
      kind: channel?.kind ?? kinds[0] ?? "",
      workspace: channel?.workspace ?? "",
      deliveryMode: channel?.deliveryMode ?? "http",
      token: "",
      appToken: "",
      signingSecret: "",
    },
  });

  const credential = CREDENTIAL[form.watch("kind")] ?? {
    label: "channels.credential",
    hint: "",
  };
  const deliveryMode = form.watch("deliveryMode");

  async function submit(values: z.infer<typeof schema>) {
    try {
      await save.mutateAsync({
        name: values.name.trim(),
        kind: values.kind as "slack",
        workspace: values.workspace.trim() || undefined,
        deliveryMode: values.deliveryMode,
        token: values.token.trim() || undefined,
        appToken: values.appToken.trim() || undefined,
        signingSecret: values.signingSecret.trim() || undefined,
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
              name="kind"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("channels.kind")}</FormLabel>
                  {/* Offered by the process rather than listed here. A console
                      that decided for itself which vendors exist would offer
                      one the binary cannot build, and the failure would arrive
                      as a connection that saves cleanly and never delivers. */}
                  <Select
                    onValueChange={field.onChange}
                    value={field.value}
                    disabled={channel !== null}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder={t("channels.kind")} />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {kinds.map((kind) => (
                        <SelectItem key={kind} value={kind}>
                          {t(`channels.vendor.${kind}`)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
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
              name="deliveryMode"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("channels.deliveryMode")}</FormLabel>
                  <Select onValueChange={field.onChange} value={field.value}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value="http">
                        {t("channels.deliveryHttp")}
                      </SelectItem>
                      <SelectItem value="socket">
                        {t("channels.deliverySocket")}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {deliveryMode === "socket"
                      ? t("channels.socketExplains")
                      : t("channels.httpExplains")}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="token"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t(credential.label)}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type="password"
                      autoComplete="off"
                      placeholder={
                        channel?.hasCredential
                          ? t("channels.tokenStored")
                          : credential.hint
                      }
                    />
                  </FormControl>
                  <FormDescription>{t("channels.tokenSealed")}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            {deliveryMode === "http" && (
              <FormField
                control={form.control}
                name="signingSecret"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("channels.signingSecret")}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type="password"
                        autoComplete="off"
                        placeholder={
                          channel?.hasSigning
                            ? t("channels.tokenStored")
                            : t("channels.signingSecretHint")
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t("channels.signingSecretExplains")}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
            {deliveryMode === "socket" && (
              <FormField
                control={form.control}
                name="appToken"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("channels.appToken")}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type="password"
                        autoComplete="off"
                        placeholder={
                          channel?.hasAppToken
                            ? t("channels.tokenStored")
                            : t("channels.appTokenHint")
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t("channels.appTokenExplains")}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

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
