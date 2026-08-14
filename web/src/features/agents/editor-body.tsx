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
  };
  tools: { catalogue: Tool[]; policies: Policy[] };
  /** Moving to the tab where the instructions are read as stages. */
  onSteps: () => void;
}) {
  // Where a column is the right answer: prose and forms are read left to
  // right, and a measure nobody can read is a measure nobody reads twice. A
  // canvas and a catalogue are not read that way and take the width.
  // Reading tabs scroll their own column; filling tabs scroll the list
  // inside them. Either way exactly one thing on the screen scrolls.
  const column =
    "mx-auto flex w-full max-w-[820px] flex-col gap-4 overflow-y-auto px-5 pt-6 pb-10";

  return (
    <>
      {/* Only while writing the first version, and only on the tab it fills
          in: offering a starting point beside an agent that already exists
          would be offering to overwrite it. */}
      {editing.creating && tab === "definition" && (
        <div className={column}>
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
        </div>
      )}

      {tab === "definition" && (
        <div className={column}>
          <TabDefinition
            draft={draft}
            patch={patch}
            editing={editing}
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
