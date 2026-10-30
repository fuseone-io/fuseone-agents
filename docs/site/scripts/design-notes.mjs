// Which design notes the site publishes, in order, with their descriptions.
// Shared by the generator and by the check that reads its output.
export const designDocs = [
  ["PRD-001-fuseone-agents.md", "Product requirements and product vocabulary."],
  ["DP-001-data-protection.md", "Stored data, retention, erasure and boundaries."],
  ["OP-001-running-an-installation.md", "Installation and operation guide."],
  ["NT-001-integration-boundary-and-execution-model.md", "Where MCP ends and integration begins."],
  ["NT-002-remaining-work.md", "Remaining product work and ordering."],
  ["NT-003-conversational-authoring.md", "Authoring an agent by conversation."],
  ["NT-004-ledger-volume-and-paging.md", "Ledger volume, partitions and paging."],
  ["NT-005-interaction-channels.md", "Channels and their product boundaries."],
  ["NT-006-evaluating-agents.md", "Evaluating agents and simulation strategy."],
  ["NT-007-drawing-a-process.md", "Process canvas and authored stages."],
  ["NT-007-standing-approvals.md", "Standing approvals for agents that act on untrusted input."],
  ["NT-008-a-catalogue-by-shape.md", "Tool catalogue chosen by shape."],
  ["NT-009-governed-connectors.md", "Governed connector shapes and runtime boundary."],
  ["NT-010-the-shape-of-the-platform.md", "Topology, the run loop, layering and where data is written."],
  ["NT-011-durable-agent-execution-and-workflow-engines.md", "Durable agent execution and the workflow-engine boundary."],
  ["NT-012-governed-slack-tickets.md", "Governed Slack tickets and Gravitee subscription approval."],
  ["NT-013-software-factory-pilot.md", "A software factory of governed agent roles, piloted on this repository."],
];

// Design notes left off the site on purpose. Leaving one out is a decision,
// so it is written here rather than inferred from the list above. A note
// listed here stays in the repository, and links to it point there.
//
// NT-002 is an ordering of remaining work written at a point in time; on an
// indexed site it reads as a current roadmap and a product commitment.
export const unpublishedDesignDocs = new Set(["NT-002-remaining-work.md"]);
