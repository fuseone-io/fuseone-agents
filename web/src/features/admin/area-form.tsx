import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
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
import { AreaFormFields } from "@/features/admin/area-form-fields";
import type { AreaFormValues } from "@/features/admin/area-form-fields";
import { useRegisterScope } from "@/features/scope/api";
import type { RegisteredScope } from "@/features/scope/api";
import { problemMessage } from "@/lib/api/problem-message";

const schema: z.ZodType<AreaFormValues> = z.object({
  company: z.string().min(1, "admin.sayCompany"),
  name: z.string().min(1, "admin.areaNeedsName"),
  label: z.string(),
});

/**
 * Declares an area.
 *
 * The name is folded server-side and the reply carries the id it became, which
 * is what the toast reports: somebody who types "Risco de Crédito" has to be
 * told the platform will call it `risco-de-credito`, because that is the
 * string they will type into a ceiling and read in a policy.
 */
export function AreaForm({
  area,
  companyOptions,
  onClose,
}: {
  area?: RegisteredScope;
  companyOptions: string[];
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const register = useRegisterScope();
  const editing = Boolean(area);

  const form = useForm<AreaFormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      company: area?.company ?? companyOptions[0] ?? "",
      name: area?.area ?? "",
      label: area?.label ?? "",
    },
  });

  useEffect(() => {
    const first = companyOptions[0];
    if (editing || form.getValues("company") || !first) {
      return;
    }
    form.setValue("company", first);
  }, [companyOptions, editing, form]);

  async function submit(values: AreaFormValues) {
    try {
      const created = await register.mutateAsync({
        company: values.company.trim(),
        name: values.name.trim(),
        label: values.label.trim() || undefined,
      });
      toast.success(
        t(editing ? "admin.areaUpdated" : "admin.areaDeclared", {
          area: created.label || created.area,
        }),
        {
          description: t("admin.areaCalled", {
            scope: `${created.company}/${created.area}`,
          }),
        },
      );
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
            {t(editing ? "admin.editArea" : "admin.newArea")}
          </DialogTitle>
          <DialogDescription>
            {t(editing ? "admin.editAreaExplains" : "admin.areaExplains")}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(submit)}
            className="flex flex-col gap-4"
          >
            <AreaFormFields
              control={form.control}
              companyOptions={companyOptions}
              editing={editing}
            />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={register.isPending}>
                {t(editing ? "common.save" : "admin.declare")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
