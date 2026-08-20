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
import { useSetPassword, type Person } from "@/features/admin/people-api";
import { problemMessage } from "@/lib/api/problem-message";

const schema = z.object({
  username: z
    .string()
    .regex(/^[A-Za-z0-9._-]*$/, "people.usernameShape"),
  password: z.string().min(12, "people.passwordFloor"),
});

/**
 * Giving somebody a password, and a handle if they have none.
 *
 * The handle is the half that matters for the administrator the setup token
 * created: they exist, they hold everything, and there is nothing to type
 * their name into — so a password alone would be a credential with no way to
 * present it.
 *
 * Sessions already open are left alone. Setting a password is ordinarily
 * somebody setting one for the first time, and signing them out of the
 * browser they are typing in would be a surprise.
 */
export function PasswordDialog({
  person,
  onClose,
}: {
  person: Person;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const set = useSetPassword();

  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { username: person.username ?? "", password: "" },
  });

  async function submit(values: z.infer<typeof schema>) {
    try {
      await set.mutateAsync({
        principalId: person.id,
        password: values.password,
        username: values.username.trim() || undefined,
      });
      toast.success(t("people.passwordSet"));
      onClose();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={t("people.setPassword")}
      description={t("people.setPasswordExplains", { name: person.display })}
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
                    <Input
                      {...field}
                      type="password"
                      autoComplete="new-password"
                    />
                  </FormControl>
                  <FormDescription>{t("people.passwordHint")}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </PropertiesSheetBody>

          <PropertiesSheetFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={set.isPending}>
              {t("people.setPassword")}
            </Button>
          </PropertiesSheetFooter>
        </form>
      </Form>
    </PropertiesSheet>
  );
}
