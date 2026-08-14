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
import { useCreateCompany } from "@/features/companies/api";
import { problemMessage } from "@/lib/api/problem-message";

const schema = z.object({
  id: z
    .string()
    .min(1, "companies.needsId")
    .regex(/^[a-z0-9]+(-[a-z0-9]+)*$/, "companies.idShape"),
  label: z.string(),
});

/**
 * Registering a company.
 *
 * The identifier is fixed once and the name is not. The identifier reaches a
 * URL, a settings key and every scope written as "company/area", so changing
 * it would take every run, grant and policy that names it along; the name is
 * what people read and people rename things.
 *
 * Creating one grants you inside it, in the same act — otherwise the screen
 * would report success and then show you nothing, because every listing here
 * is filtered by the scopes you hold.
 */
export function CompanyForm({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const create = useCreateCompany();

  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { id: "", label: "" },
  });

  async function submit(values: z.infer<typeof schema>) {
    try {
      await create.mutateAsync({
        id: values.id.trim(),
        label: values.label.trim() || undefined,
      });
      toast.success(t("companies.registered"), {
        description: t("companies.grantedYou"),
      });
      onClose();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("companies.register")}</DialogTitle>
          <DialogDescription>{t("companies.explains")}</DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(submit)}
            className="flex flex-col gap-4"
          >
            <FormField
              control={form.control}
              name="id"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("companies.identifier")}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      className="font-mono"
                      placeholder="acme"
                    />
                  </FormControl>
                  <FormDescription>
                    {t("companies.idNeverChanges")}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="label"
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

            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={create.isPending}>
                {t("companies.register")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
