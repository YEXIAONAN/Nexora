import { requestJSON } from './client'
import { projectPath } from './projects'
import type { components } from './schema'

type S = components['schemas']
export type BonusResult = S['BonusResult']
export type BonusRoundVersion = S['BonusRoundVersion']
export type BonusRoundSummary = S['BonusRoundSummary']
export type BonusRoundDetail = S['BonusRoundDetail']

export type BonusRoundStatus = BonusRoundSummary['status']
export type BonusVersionStatus = BonusRoundVersion['status']

export const bonusRoundsPath = (team: string, project: string, roundID?: string) =>
  `${projectPath(team, project)}/bonus-rounds${roundID ? `/${encodeURIComponent(roundID)}` : ''}`

async function read<T>(path: string, signal?: AbortSignal): Promise<T> {
  return (await requestJSON<{ data: T }>(path, { signal })).data
}

/** Read-only funding-round metadata for one project. Amounts are never included. */
export function listBonusRounds(team: string, project: string, signal?: AbortSignal) {
  return read<{ items: BonusRoundSummary[]; has_more: boolean }>(
    bonusRoundsPath(team, project),
    signal,
  )
}

/** One funding round plus its immutable plan-version history. */
export function getBonusRound(
  team: string,
  project: string,
  roundID: string,
  signal?: AbortSignal,
) {
  return read<BonusRoundDetail>(bonusRoundsPath(team, project, roundID), signal)
}

/** The signed-in subject's effective confirmed bonus results for one team. */
export function listMyBonusResults(team: string, signal?: AbortSignal) {
  return read<{ items: BonusResult[]; has_more: boolean }>(
    `/api/v1/teams/${encodeURIComponent(team)}/bonus-results/mine`,
    signal,
  )
}
