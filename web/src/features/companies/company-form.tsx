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
import { Form } from "@/components/ui/form";
import {
  useCreateCompany,
  useUpdateCompany,
  type Company,
} from "@/features/companies/api";
import { CompanyFormFields } from "@/features/companies/company-form-fields";
import type { CompanyFormValues } from "@/features/companies/company-form-fields";
import { problemMessage } from "@/lib/api/problem-message";

const schema: z.ZodType<CompanyFormValues> = z.object({
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
export function CompanyForm({
  company,
  onClose,
}: {
  company?: Company;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const create = useCreateCompany();
  const update = useUpdateCompany();
  const editing = Boolean(company);

  const form = useForm<CompanyFormValues>({
    resolver: zodResolver(schema),
    defaultValues: { id: company?.id ?? "", label: company?.label ?? "" },
  });

  async function submit(values: CompanyFormValues) {
    try {
      if (company) {
        await update.mutateAsync({
          company: company.id,
          label: values.label.trim() || company.id,
        });
        toast.success(t("companies.saved"));
      } else {
        await create.mutateAsync({
          id: values.id.trim(),
          label: values.label.trim() || undefined,
        });
        toast.success(t("companies.registered"), {
          description: t("companies.grantedYou"),
        });
      }
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
            {t(editing ? "companies.editing" : "companies.register")}
          </DialogTitle>
          <DialogDescription>
            {t(editing ? "companies.editExplains" : "companies.explains")}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(submit)}
            className="flex flex-col gap-4"
          >
            <CompanyFormFields control={form.control} editing={editing} />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                {t("common.cancel")}
              </Button>
              <Button
                type="submit"
                disabled={create.isPending || update.isPending}
              >
                {t(editing ? "common.save" : "companies.register")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
