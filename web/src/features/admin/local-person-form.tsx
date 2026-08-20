import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
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
import { useCreateLocalPerson } from "@/features/admin/people-api";
import { problemMessage } from "@/lib/api/problem-message";

const schema = z.object({
  username: z
    .string()
    .min(3, "people.usernameShape")
    .regex(/^[A-Za-z0-9._-]+$/, "people.usernameShape"),
  password: z.string().min(12, "people.passwordFloor"),
  display: z.string(),
});

/**
 * Somebody who signs in with a password.
 *
 * They arrive holding nothing: granting is the next act, on the same screen,
 * so creating a person and deciding what they may do stay two decisions and
 * two entries in the trail.
 */
export function LocalPersonForm({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const create = useCreateLocalPerson();

  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { username: "", password: "", display: "" },
  });

  async function submit(values: z.infer<typeof schema>) {
    try {
      await create.mutateAsync({
        username: values.username.trim(),
        password: values.password,
        display: values.display.trim() || undefined,
      });
      toast.success(t("people.created"), { description: t("people.holdsNothing") });
      onClose();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={t("people.addLocal")}
      description={t("people.localExplains")}
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(submit)}
          className="flex min-h-0 flex-1 flex-col"
        >
          <PropertiesSheetBody className="space-y-4">
            <FormField
              control={form.control}
              name="username"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("people.username")}</FormLabel>
                  <FormControl>
                    <Input {...field} className="font-mono" autoComplete="off" />
                  </FormControl>
                  <FormDescription>{t("people.usernameHint")}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="password"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("people.password")}</FormLabel>
                  <FormControl>
                    <Input {...field} type="password" autoComplete="new-password" />
                  </FormControl>
                  <FormDescription>{t("people.passwordHint")}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="display"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("admin.shownAs")}</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder={t("common.optional")} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </PropertiesSheetBody>

          <PropertiesSheetFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={create.isPending}>
              {t("people.addLocal")}
            </Button>
          </PropertiesSheetFooter>
        </form>
      </Form>
    </PropertiesSheet>
  );
}
