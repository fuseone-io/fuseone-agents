import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { z } from "zod";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import { Button } from "@/components/ui/button";
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
import { Textarea } from "@/components/ui/textarea";
import { useCreateServiceAccount } from "@/features/admin/service-accounts-api";
import { problemMessage } from "@/lib/api/problem-message";

const schema = z.object({
  name: z.string().regex(/^[a-z][a-z0-9-]{0,62}$/, "serviceAccounts.nameShape"),
  description: z.string(),
});

type Values = z.infer<typeof schema>;

/** The account arrives holding nothing: granting is a separate act and line. */
export function ServiceAccountForm({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const create = useCreateServiceAccount();
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { name: "", description: "" },
  });

  async function submit(values: Values) {
    try {
      await create.mutateAsync({
        name: values.name,
        description: values.description.trim() || undefined,
      });
      toast.success(t("serviceAccounts.created", { name: values.name }));
      onClose();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={t("serviceAccounts.create")}
      description={t("serviceAccounts.createExplains")}
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(submit)}
          className="flex min-h-0 flex-1 flex-col"
        >
          <PropertiesSheetBody className="space-y-4">
            <FormField
              control={form.control}
              name="name"
              render={({ field, fieldState }) => (
                <FormItem>
                  <FormLabel>{t("serviceAccounts.name")}</FormLabel>
                  <FormControl>
                    <Input {...field} className="font-mono" autoComplete="off" />
                  </FormControl>
                  {!fieldState.error && (
                    <FormDescription>
                      {t("serviceAccounts.nameHint")}
                    </FormDescription>
                  )}
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="description"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("serviceAccounts.description")}</FormLabel>
                  <FormControl>
                    <Textarea {...field} className="min-h-20" />
                  </FormControl>
                  <FormDescription>
                    {t("serviceAccounts.descriptionHint")}
                  </FormDescription>
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
              {t("serviceAccounts.createConfirm")}
            </Button>
          </PropertiesSheetFooter>
        </form>
      </Form>
    </PropertiesSheet>
  );
}
