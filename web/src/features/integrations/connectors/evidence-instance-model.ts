import type {
  ConnectorEvidenceConfig,
  ConnectorInstanceDetail,
  ConnectorInstanceInput,
} from "@/features/integrations/api";
import {
  applyConnectorScope,
  type ConnectorInstanceSaveInput,
} from "@/features/integrations/connectors/connector-instance-model";
import {
  evidenceList,
  type EvidenceInstanceValues,
  type EvidenceQueryValues,
} from "@/features/integrations/connectors/evidence-instance-schema";

export { parseEvidenceDeclaration } from "@/features/integrations/connectors/evidence-declaration-schema";
export {
  evidenceControlEligible,
  evidenceInstanceSchema,
  type EvidenceInstanceValues,
  type EvidenceQueryValues,
} from "@/features/integrations/connectors/evidence-instance-schema";

type EvidenceQuery = ConnectorEvidenceConfig["queries"][number];
type DeclarationValues = Omit<
  EvidenceInstanceValues,
  "name" | "enabled" | "scopeKind" | "company" | "area" | "token" | "clearToken"
>;

export function emptyEvidenceQuery(): EvidenceQueryValues {
  return {
    id: "",
    description: "",
    path: "",
    filters: [],
    parameters: [],
    fields: "",
    idField: "",
    timeField: "",
    windowless: false,
  };
}

const emptyDeclaration: DeclarationValues = {
  baseUrl: "",
  authHeader: "",
  authPrefix: "",
  control: "",
  rows: "",
  refusalField: "",
  refusalValue: "",
  gapField: "",
  gapValue: "",
  maxEvents: 0,
  pagingKind: "offset",
  limitParam: "",
  pageSize: 1000,
  offsetParam: "",
  cursorParam: "",
  nextCursor: "",
  windowKind: "range",
  periodParam: "",
  periods: "",
  startParam: "",
  endParam: "",
  format: "iso",
  queries: [emptyEvidenceQuery()],
};

/** What a stored or pasted declaration looks like as form fields. */
export function evidenceDeclarationValues(
  config: ConnectorEvidenceConfig,
): DeclarationValues {
  return {
    baseUrl: config.baseUrl,
    authHeader: config.authHeader,
    authPrefix: config.authPrefix ?? "",
    control: config.control,
    rows: config.rows,
    refusalField: config.refusal?.field ?? "",
    refusalValue: config.refusal?.value ?? "",
    gapField: config.gap?.field ?? "",
    gapValue: config.gap?.value ?? "",
    maxEvents: config.maxEvents ?? 0,
    pagingKind: config.paging.kind,
    limitParam: config.paging.limitParam,
    pageSize: config.paging.pageSize,
    offsetParam: config.paging.offsetParam ?? "",
    cursorParam: config.paging.cursorParam ?? "",
    nextCursor: config.paging.nextCursor ?? "",
    windowKind: config.window.kind,
    periodParam: config.window.periodParam ?? "",
    periods: (config.window.periods ?? []).join(", "),
    startParam: config.window.startParam ?? "",
    endParam: config.window.endParam ?? "",
    format: config.window.format ?? "iso",
    queries: config.queries.map(queryValues),
  };
}

function queryValues(query: EvidenceQuery): EvidenceQueryValues {
  return {
    id: query.id,
    description: query.description,
    path: query.path,
    filters: (query.filters ?? []).map((filter) => ({ ...filter })),
    parameters: (query.parameters ?? []).map((parameter) => ({ ...parameter })),
    fields: query.fields.join(", "),
    idField: query.idField ?? "",
    timeField: query.timeField ?? "",
    windowless: query.windowless ?? false,
  };
}

export function evidenceInstanceDefaults(
  instance: ConnectorInstanceDetail | null,
  company: string,
  area: string,
): EvidenceInstanceValues {
  return {
    name: instance?.name ?? "",
    enabled: instance?.enabled ?? true,
    scopeKind: instance?.scopeKind ?? "area",
    company: instance?.company ?? company,
    area: instance?.area ?? area,
    ...(instance?.evidence
      ? evidenceDeclarationValues(instance.evidence)
      : emptyDeclaration),
    token: "",
    clearToken: false,
  };
}

function queryDeclaration(query: EvidenceQueryValues): EvidenceQuery {
  return {
    id: query.id,
    description: query.description,
    path: query.path,
    fields: evidenceList(query.fields),
    ...(query.filters.length > 0 && { filters: query.filters }),
    ...(query.parameters.length > 0 && { parameters: query.parameters }),
    ...(query.idField && { idField: query.idField }),
    ...(query.timeField && { timeField: query.timeField }),
    ...(query.windowless && { windowless: true }),
  };
}

function pagingDeclaration(
  values: EvidenceInstanceValues,
): ConnectorEvidenceConfig["paging"] {
  const shared = {
    kind: values.pagingKind,
    limitParam: values.limitParam,
    pageSize: values.pageSize,
  };
  return values.pagingKind === "offset"
    ? { ...shared, offsetParam: values.offsetParam }
    : {
        ...shared,
        cursorParam: values.cursorParam,
        nextCursor: values.nextCursor,
      };
}

function windowDeclaration(
  values: EvidenceInstanceValues,
): ConnectorEvidenceConfig["window"] {
  return values.windowKind === "period"
    ? {
        kind: "period",
        periodParam: values.periodParam,
        periods: evidenceList(values.periods).map(Number),
      }
    : {
        kind: "range",
        startParam: values.startParam,
        endParam: values.endParam,
        format: values.format,
      };
}

function declaration(values: EvidenceInstanceValues): ConnectorEvidenceConfig {
  return {
    baseUrl: values.baseUrl,
    authHeader: values.authHeader,
    ...(values.authPrefix && { authPrefix: values.authPrefix }),
    control: values.control,
    rows: values.rows,
    ...(values.refusalField && {
      refusal: { field: values.refusalField, value: values.refusalValue },
    }),
    ...(values.gapField && {
      gap: { field: values.gapField, value: values.gapValue },
    }),
    ...(values.maxEvents > 0 && { maxEvents: values.maxEvents }),
    paging: pagingDeclaration(values),
    window: windowDeclaration(values),
    queries: values.queries.map(queryDeclaration),
  };
}

/** Null when an enabled source would be left with no token to read with. */
export function evidenceInstancePayload(
  values: EvidenceInstanceValues,
  hasStoredToken: boolean,
): ConnectorInstanceSaveInput | null {
  const newToken = values.token.trim() !== "";
  const keepsToken = hasStoredToken && !values.clearToken;
  if (values.enabled && !newToken && !keepsToken) return null;

  const body: ConnectorInstanceInput = {
    connector: "evidence",
    enabled: values.enabled,
    scopeKind: values.scopeKind,
    evidence: declaration(values),
  };
  applyConnectorScope(body, values);
  if (newToken) body.token = values.token;
  if (values.clearToken && !newToken) body.clearToken = true;
  return { name: values.name.trim(), body };
}
