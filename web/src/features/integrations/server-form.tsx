import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
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
import { usePutMCPServer, type MCPServer } from "@/features/integrations/api";

const schema = z.object({
  name: z
    .string()
    .min(1, "Dê um nome ao servidor.")
    .regex(/^[a-z0-9][a-z0-9_-]*$/, "Minúsculas, números, hífen e sublinhado."),
  command: z.string().min(1, "Diga o que executar."),
  args: z.string(),
  enabled: z.boolean(),
});

export function ServerForm({
  server,
  onClose,
}: {
  server: MCPServer | null;
  onClose: () => void;
}) {
  const put = usePutMCPServer();
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: server?.name ?? "",
      command: server?.command ?? "",
      args: (server?.args ?? []).join(" "),
      enabled: server?.enabled ?? true,
    },
  });

  async function submit(values: z.infer<typeof schema>) {
    try {
      await put.mutateAsync({
        name: values.name,
        command: values.command,
        args: values.args.split(/\s+/).filter(Boolean),
        enabled: values.enabled,
      });
      toast.success(`${values.name} configurado`, {
        // Servers are connected when a worker starts, so saying it is live
        // would be a promise the platform does not keep yet.
        description:
          "As ferramentas aparecem quando um worker reiniciar e conectar.",
      });
      onClose();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Não foi possível salvar",
      );
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {server ? "Editar servidor" : "Novo servidor de ferramentas"}
          </DialogTitle>
          <DialogDescription>
            Um servidor MCP é o que dá ferramentas aos agentes. Tudo que ele
            oferece chega como leitura até alguém classificar.
          </DialogDescription>
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
                  <FormLabel>Nome</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      disabled={!!server}
                      className="font-mono"
                    />
                  </FormControl>
                  <FormDescription>
                    Prefixa as ferramentas:{" "}
                    <code className="font-mono">crm</code> dá{" "}
                    <code className="font-mono">crm.lookup</code>.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="command"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Comando</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      className="font-mono"
                      placeholder="/usr/local/bin/crm-mcp"
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="args"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Argumentos</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      className="font-mono"
                      placeholder="--config /etc/crm.yaml"
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="enabled"
              render={({ field }) => (
                <FormItem className="flex items-center justify-between rounded-lg border p-3">
                  <FormLabel className="m-0">Ativo</FormLabel>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                Cancelar
              </Button>
              <Button type="submit" disabled={form.formState.isSubmitting}>
                Salvar
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
