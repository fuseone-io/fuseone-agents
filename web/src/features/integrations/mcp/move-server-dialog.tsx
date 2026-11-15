import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Form, FormControl, FormField, FormItem, FormLabel } from "@/components/ui/form";
import { CompanyAreaFields } from "@/features/scope/company-area-fields";
import { ScopeListsNotice } from "@/features/scope/scope-lists-notice";
import { useMoveMCPServer } from "@/features/integrations/mcp/move-api";
import { moveRefusal, type StrandedAgent } from "@/features/integrations/mcp/move-refusal";
import { StrandedAgents } from "@/features/integrations/mcp/stranded-agents";
import { isStale } from "@/lib/api/revision";

const schema = z.object({
  scope: z.object({ company: z.string().min(1, "mcp.move.companyRequired"), area: z.string() }),
  strand: z.boolean(),
});
type MoveValues = z.infer<typeof schema>;

const pathOf = (s: { company: string; area: string }) => (s.area ? `${s.company}/${s.area}` : s.company);

/**
 * Moves a server of the installation to the company or area that owns it
 * (NT-019). The platform's act alone, against the revision the page opened
 * on: after a 412 nothing more is sent, and the page's reload brings a new
 * revision and a new dialog. When published agents outside the target name
 * the server's tools, they are listed, and the move goes ahead only once
 * somebody says to strand them, for that target.
 */
export function MoveServerDialog({
  name,
  etag,
  onMoved,
  onStale,
}: {
  name: string;
  etag: string;
  onMoved: () => void;
  onStale: () => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [stranded, setStranded] = useState<StrandedAgent[]>([]);
  const [refusal, setRefusal] = useState<string>();
  const [stale, setStale] = useState(false);
  const move = useMoveMCPServer();
  const form = useForm<MoveValues>({
    resolver: zodResolver(schema),
    defaultValues: { scope: { company: "", area: "" }, strand: false },
  });

  async function submit(values: MoveValues) {
    setRefusal(undefined);
    try {
      await move.mutateAsync({ name, etag, ...values.scope, strand: values.strand });
      toast.success(t("mcp.move.moved", { scope: pathOf(values.scope) }));
      setOpen(false);
      onMoved();
    } catch (problem) {
      if (isStale(problem)) {
        setStale(true);
        onStale();
      }
      const answer = moveRefusal(problem, t, pathOf(values.scope));
      if ("stranded" in answer) setStranded(answer.stranded);
      else setRefusal(answer.message);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline">{t("mcp.move.action")}</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("mcp.move.title", { name })}</DialogTitle>
          <DialogDescription>{t("mcp.move.description")}</DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form id="move-server" onSubmit={form.handleSubmit(submit)} className="space-y-4">
            <FormField
              control={form.control}
              name="scope"
              render={({ field }) => (
                <div className="grid gap-3 sm:grid-cols-2">
                  <CompanyAreaFields
                    value={field.value}
                    shape={{
                      idPrefix: "move",
                      errors: { company: form.formState.errors.scope?.company?.message },
                    }}
                    onChange={(next) => {
                      // Agreeing to strand one target's agents is no word on
                      // another's, and a refusal speaks of the target that drew
                      // it. A stale revision is no target's: it stays.
                      setStranded([]);
                      form.setValue("strand", false);
                      if (!stale) setRefusal(undefined);
                      field.onChange(next);
                    }}
                  />
                </div>
              )}
            />
            <ScopeListsNotice message={t("scope.listsFailed")} />
            {stranded.length > 0 && (
              <>
                <StrandedAgents agents={stranded} />
                <FormField
                  control={form.control}
                  name="strand"
                  render={({ field }) => (
                    <FormItem className="flex items-start gap-2">
                      <FormControl>
                        <Checkbox checked={field.value} onCheckedChange={(v) => field.onChange(v === true)} />
                      </FormControl>
                      <FormLabel className="font-normal">{t("mcp.move.strand")}</FormLabel>
                    </FormItem>
                  )}
                />
              </>
            )}
            {refusal && <p role="alert" className="text-sm text-danger">{refusal}</p>}
          </form>
        </Form>
        <DialogFooter>
          <Button type="submit" form="move-server" disabled={move.isPending || stale}>
            {t("mcp.move.submit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
