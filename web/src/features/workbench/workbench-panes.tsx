import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

/**
 * The conversation on the left and the workspace on the right, filling the
 * screen's height, each scrolling on its own. On a narrow screen the two are
 * tabs, so neither is a long scroll below the other. Both stay mounted: what
 * the person was writing survives switching to the draft and back.
 */
export function WorkbenchPanes({ conversation, workspace }: { conversation: ReactNode; workspace: ReactNode }) {
  const { t } = useTranslation();
  const pane = "min-h-[32rem] flex-1 data-[state=inactive]:hidden lg:min-h-0 lg:data-[state=inactive]:block";

  return (
    <Tabs
      defaultValue="conversation"
      className="flex min-h-0 flex-1 flex-col gap-3 lg:grid lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)] lg:gap-6"
    >
      <TabsList className="lg:hidden">
        <TabsTrigger value="conversation">{t("workbench.conversation.title")}</TabsTrigger>
        <TabsTrigger value="workspace">{t("workbench.workspace")}</TabsTrigger>
      </TabsList>
      <TabsContent value="conversation" forceMount className={pane}>
        {conversation}
      </TabsContent>
      <TabsContent value="workspace" forceMount className={pane}>
        {workspace}
      </TabsContent>
    </Tabs>
  );
}
