import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { FilePlus2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { PageHeader } from "@/components/shared/page-header";
import { PAGE_ICONS } from "@/components/layout/nav";
import { useCreateDraft } from "@/features/workbench/api";
import { WorkbenchIdentityFields, type IdentityValues } from "@/features/workbench/workbench-identity-fields";
import { problemMessage } from "@/lib/api/problem-message";

const schema = z.object({
  agentId: z.string().trim().min(1, "workbench.idRequired"),
  name: z.string().trim().min(1, "workbench.nameRequired"),
  company: z.string().min(1, "workbench.areaRequired"),
  area: z.string().min(1, "workbench.areaRequired"),
}) satisfies z.ZodType<IdentityValues>;

/**
 * /agents/workbench: a new agent's birth (NT-018, contract 1). The first step
 * is a person's: identifier, name and scope. The draft is opened with
 * If-None-Match: *, so an identifier already drafted or published is refused,
 * and the identifier is fixed from then on.
 */
export function WorkbenchNewPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const create = useCreateDraft();
  const form = useForm<IdentityValues>({
    resolver: zodResolver(schema),
    defaultValues: { agentId: "", name: "", company: "", area: "" },
  });

  const submit = (values: IdentityValues) => {
    const agentId = values.agentId.trim();
    create.mutate(
      { agentId, identity: { name: values.name.trim(), company: values.company, area: values.area } },
      {
        onSuccess: () => void navigate(`/agents/${agentId}/workbench`),
        onError: (error) => toast.error(problemMessage(error, t)),
      },
    );
  };

  return (
    <div className="flex flex-col gap-6">
      <PageHeader icon={PAGE_ICONS.agents} title={t("workbench.newTitle")} description={t("workbench.newHint")} />
      <Form {...form}>
        <form onSubmit={form.handleSubmit(submit)} className="flex max-w-xl flex-col gap-4">
          <WorkbenchIdentityFields control={form.control} />
          <Button type="submit" className="self-start" disabled={create.isPending}>
            <FilePlus2 className="size-4" />
            {t("workbench.create")}
          </Button>
        </form>
      </Form>
    </div>
  );
}
