import { PageHeader } from "@/components/shared/page-header";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ToolsPanel } from "@/features/admin/tools-panel";
import { IntegrationsPanel } from "@/features/admin/integrations-panel";
import { EventsPanel } from "@/features/admin/events-panel";
import { BudgetsPanel } from "@/features/admin/budgets-panel";

/**
 * Everything an operator configures lives here, and every change made here is
 * recorded. That pairing is the point: the platform's rules are editable and
 * the edits are auditable, or neither is worth much.
 */
export function AdminPage() {
  return (
    <>
      <PageHeader
        title="Administração"
        description="O que a plataforma conversa, o que cada ferramenta faz com o mundo, e quem mudou o quê."
      />

      <Tabs defaultValue="tools" className="min-h-0 flex-1">
        <TabsList>
          <TabsTrigger value="tools">Ferramentas</TabsTrigger>
          <TabsTrigger value="integrations">Integrações</TabsTrigger>
          <TabsTrigger value="budgets">Tetos</TabsTrigger>
          <TabsTrigger value="events">Trilha</TabsTrigger>
        </TabsList>

        <TabsContent value="tools" className="mt-4">
          <ToolsPanel />
        </TabsContent>
        <TabsContent value="integrations" className="mt-4">
          <IntegrationsPanel />
        </TabsContent>
        <TabsContent value="budgets" className="mt-4">
          <BudgetsPanel />
        </TabsContent>
        <TabsContent value="events" className="mt-4">
          <EventsPanel />
        </TabsContent>
      </Tabs>
    </>
  );
}
