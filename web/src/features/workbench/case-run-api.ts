import { useRef } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { experimentKeys, type ExperimentListing, type ExperimentRequest } from "@/features/experiments/experiments-api";
import { isAbsent } from "@/features/workbench/api";

/*
Running a candidate against the cases (NT-018 phase 5c). The candidate runs on the newest
dataset version under its own configuration, and the published version is the baseline it
is compared with, on that same dataset version. Every pin is read once, when the person
asks: a version published or a dataset version written while the runs go cannot move the
baseline, because nothing is read again. Asking again after a failure reuses what the first
asking read, the absence of a published version included, until the whole run is opened.
*/

/** What one run against the cases opened: the candidate's experiment and its baseline. */
export type CaseRun = { experiment: string; baseline?: string; reused: boolean };

/** What a baseline must have pinned to be compared with the candidate. */
export type BaselinePins = { version: string; datasetVersion: number; repetitions: number };

const noConfig = (l: ExperimentListing) => !l.config.provider && !l.config.model && !l.config.effort;

/** Whether a listed experiment ran exactly the pins the baseline needs. */
export const matchesPins = (l: ExperimentListing, pins: BaselinePins) =>
  l.version === pins.version && l.datasetVersion === pins.datasetVersion && l.repetitions === pins.repetitions && noConfig(l);

/** The published version now, or none when the agent was never published. */
async function publishedVersion(agentId: string): Promise<string | undefined> {
  try {
    return unwrap(await api.GET("/agents/{agentId}", { params: { path: { agentId } } })).agent.versionId;
  } catch (error) {
    if (isAbsent(error)) return undefined;
    throw error;
  }
}

/**
 * The newest ended experiment with exactly these pins, among those the listing answers: the
 * API lists an agent's newest twenty, so an older one is not found and a new one is opened.
 * One still running is not a baseline: what it found is not yet what it will have found.
 */
async function endedBaseline(agentId: string, pins: BaselinePins, except: string): Promise<string | undefined> {
  const listed = unwrap(await api.GET("/agents/{agentId}/experiments", { params: { path: { agentId } } }));
  for (const l of listed.experiments) {
    if (l.id === except || !matchesPins(l, pins)) continue;
    const report = unwrap(await api.GET("/agents/{agentId}/experiments/{experimentId}", { params: { path: { agentId, experimentId: l.id } } }));
    if (!report.summary.running) return l.id;
  }
  return undefined;
}

/**
 * Runs a candidate against the cases and finds or opens its baseline. Each request's key is
 * kept until the whole run is opened: asking again after a lost answer reaches the
 * experiments already opened, never a second candidate or a second baseline.
 */
export function useRunAgainstCases(agentId: string) {
  const queryClient = useQueryClient();
  const intentions = useRef(new Map<string, string>());
  // What each asking read as the published version, kept as a box so "none" is kept too.
  const published = useRef(new Map<string, { version?: string }>());
  const pinnedVersion = async (asking: string) => {
    const kept = published.current.get(asking);
    if (kept) return kept.version;
    const version = await publishedVersion(agentId);
    published.current.set(asking, { version });
    return version;
  };
  const open = async (body: ExperimentRequest) => {
    const intention = JSON.stringify(body);
    const key = intentions.current.get(intention) ?? `exp-${crypto.randomUUID()}`;
    intentions.current.set(intention, key);
    return unwrap(await api.POST("/agents/{agentId}/experiments", { params: { path: { agentId }, header: { "Idempotency-Key": key } }, body }));
  };

  return useMutation({
    mutationFn: async ({ sealId, repetitions }: { sealId: string; repetitions: number }): Promise<CaseRun> => {
      const version = await pinnedVersion(JSON.stringify({ sealId, repetitions }));
      const candidate = await open({ sealId, repetitions });
      if (version === undefined) return { experiment: candidate.id, reused: false };
      const pins = { version, datasetVersion: candidate.datasetVersion, repetitions };
      const found = await endedBaseline(agentId, pins, candidate.id);
      if (found) return { experiment: candidate.id, baseline: found, reused: true };
      const opened = await open(pins);
      return { experiment: candidate.id, baseline: opened.id, reused: false };
    },
    onSuccess: () => {
      intentions.current.clear();
      published.current.clear();
      void queryClient.invalidateQueries({ queryKey: experimentKeys.all(agentId) });
    },
  });
}
