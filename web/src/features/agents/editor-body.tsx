import { TabDefinition } from "@/features/agents/tab-definition";
import { TabGovernance } from "@/features/agents/tab-governance";
import { TabSteps } from "@/features/agents/tab-steps";
import { AgentToolsSection } from "@/features/agents/agent-tools-section";
import { TemplateGallery } from "@/features/agents/template-gallery";
import type { EditorTab } from "@/features/agents/editor-tabs";
import type { AgentDefinition, Policy, Tool } from "@/lib/api/client";

/**
 * Whichever tab is open, and nothing else.
 *
 * One decision at a time is the whole point of the tabs: a field that does not
 * answer the open tab's question belongs in another tab, and a fourth card in
 * one of them is the signal to split rather than to scroll.
 */
export function EditorBody({
  tab,
  draft,
  patch,
  editing,
  tools,
  onSteps,
}: {
  tab: EditorTab;
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
  editing: {
    agentId: string;
    creating: boolean;
    onAgentId: (id: string) => void;
    template?: string;
    onTemplate: (id?: string) => void;
    /** The instruction as published, which is what a diff is against. */
    published?: string;
  };
  tools: { catalogue: Tool[]; policies: Policy[]; enabled: string[] };
  /** Moving to the tab where the instructions are read as stages. */
  onSteps: () => void;
}) {
  // Where a column is the right answer: prose and forms are read left to
  // right, and a measure nobody can read is a measure nobody reads twice. A
  // canvas and a catalogue are not read that way and take the width.
  // Reading tabs scroll their own column; filling tabs scroll the list
  // inside them. Either way exactly one thing on the screen scrolls.
  // `[&>*]:shrink-0` because a flex column shrinks its children before it
  // scrolls: with enough instruction text the identity card was compressed
  // until its middle row was sliced in half, which reads as a broken screen
  // rather than as a column that needed scrolling. Cards keep their height and
  // the column scrolls, which is what the comment above always claimed.
  const column =
    "mx-auto flex w-full max-w-[1040px] flex-col gap-4 overflow-y-auto px-5 pt-6 pb-10 [&>*]:shrink-0";

  return (
    <>
      {tab === "definition" && (
        <div data-testid="agent-definition-column" className={column}>
          {/* Only while writing the first version, and only on the tab it fills
              in: offering a starting point beside an agent that already exists
              would be offering to overwrite it. It lives in the same scroller
              as the form because they are one column; sibling scrollers split
              the page and hide whichever one lost the height negotiation. */}
          {editing.creating && (
            <TemplateGallery
              chosen={editing.template}
              onChoose={(template) => {
                patch({
                  name: template.name,
                  area: template.area ?? draft.area,
                  instructions: template.instructions,
                  triggers: template.triggers,
                  budget: template.budget ?? draft.budget,
                });
                editing.onAgentId(template.id);
                editing.onTemplate(template.id);
              }}
              onClear={() => {
                editing.onTemplate(undefined);
                patch({ name: "", instructions: "", triggers: [] });
              }}
            />
          )}
          <TabDefinition
            draft={draft}
            patch={patch}
            editing={editing}
            tools={tools}
            onSteps={onSteps}
          />
        </div>
      )}
      {tab === "steps" && (
        <TabSteps
          draft={draft}
          patch={patch}
          catalogue={tools.catalogue}
          policies={tools.policies}
        />
      )}
      {tab === "tools" && (
        <div className="flex min-h-0 flex-1 flex-col">
          <AgentToolsSection
            granted={draft.tools ?? []}
            catalogue={tools.catalogue}
            policies={tools.policies}
            patch={patch}
          />
        </div>
      )}
      {tab === "governance" && (
        <div className={column}>
          <TabGovernance draft={draft} patch={patch} />
        </div>
      )}
    </>
  );
}
