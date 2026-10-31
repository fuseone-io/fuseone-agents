import type { MCPGitHubApp } from "@/features/integrations/api";

/**
 * The GitHub App half of a credential form, as typed.
 *
 * Strings because they are inputs: an id is only a number once somebody has
 * finished typing it, and the schema is what says when that is.
 */
export type GitHubAppInput = {
  githubAppId: string;
  githubInstallationId: string;
  githubPrivateKey: string;
  githubRepositories: string;
  githubAPIBaseURL: string;
};

/** Saves an App credential and says whether it was stored. */
export type WriteGitHubApp = (credential: { githubApp: MCPGitHubApp }) => Promise<boolean>;

export function blankGitHubApp(): GitHubAppInput {
  return {
    githubAppId: "",
    githubInstallationId: "",
    githubPrivateKey: "",
    githubRepositories: "",
    githubAPIBaseURL: "",
  };
}

export function githubAppHasValue(value: GitHubAppInput) {
  return [
    value.githubAppId,
    value.githubInstallationId,
    value.githubPrivateKey,
    value.githubRepositories,
    value.githubAPIBaseURL,
  ].some((part) => part.trim() !== "");
}

/** Both ids are whole numbers above zero, or the App is not filled in at all. */
export function githubAppIdsValid(value: GitHubAppInput) {
  if (!githubAppHasValue(value)) return true;
  return wholeNumber(value.githubAppId) && wholeNumber(value.githubInstallationId);
}

/**
 * A new App needs its key. A stored one keeps its key when the field is left
 * blank, so correcting an installation does not demand pasting it again.
 */
export function githubAppComplete(value: GitHubAppInput, hasStoredKey: boolean) {
  return (
    githubAppHasValue(value) &&
    githubAppIdsValid(value) &&
    (hasStoredKey || value.githubPrivateKey.trim() !== "")
  );
}

/** The request shape. A blank key is omitted, which keeps the stored one. */
export function githubAppFromValue(value: GitHubAppInput): MCPGitHubApp {
  const repositories = value.githubRepositories
    .split(/[\s,]+/)
    .map((repo) => repo.trim())
    .filter(Boolean);
  const key = value.githubPrivateKey.trim();
  const base = value.githubAPIBaseURL.trim();
  return {
    appId: Number.parseInt(value.githubAppId.trim(), 10),
    installationId: Number.parseInt(value.githubInstallationId.trim(), 10),
    privateKey: key === "" ? undefined : key,
    repositories: repositories.length === 0 ? undefined : repositories,
    apiBaseURL: base === "" ? undefined : base,
  };
}

/**
 * Reads an uploaded key file. FileReader rather than Blob.text(): the same
 * result in every browser the console supports, and in the test DOM.
 */
export function readKeyFile(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? "").trim());
    reader.onerror = () => reject(reader.error);
    reader.readAsText(file);
  });
}

function wholeNumber(raw: string) {
  return /^\d+$/.test(raw.trim()) && Number(raw) > 0;
}
