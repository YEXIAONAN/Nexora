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

export type BonusMember = S['BonusMember']
export type BonusSource = S['BonusSource']
export type BonusProjectWorkspace = S['BonusProjectWorkspace']
export type BonusPlanLine = S['BonusPlanLine']
export type BonusPlanSource = S['BonusPlanSource']
export type BonusPlan = S['BonusPlan']
export type BonusAction = Exclude<keyof S['BonusPermissions'], 'edit' | 'record_payment'>
export type BonusGrantRole = S['BonusGrantRole']
export type BonusWorkspace = S['BonusWorkspace']
export type BonusCommand = S['BonusCommand']
export type BonusCommandResult = S['BonusCommandResult']
export type CreateBonusRound = S['CreateBonusRound']
export type BonusPlanInput = Omit<S['BonusPlanInput'], keyof BonusCommand>
export type BonusGrantInput = Omit<S['BonusGrantInput'], keyof BonusCommand>
export type BonusPaymentInput = Omit<S['BonusPaymentInput'], keyof BonusCommand>

export const bonusRoundsPath = (team: string, project: string, roundID?: string) =>
  `${projectPath(team, project)}/bonus-rounds${roundID ? `/${encodeURIComponent(roundID)}` : ''}`

async function read<T>(path: string, signal?: AbortSignal): Promise<T> {
  return (await requestJSON<{ data: T }>(path, { signal })).data
}
async function write(path: string, body: unknown, signal?: AbortSignal, method = 'POST') {
  return (await requestJSON<{ data: BonusCommandResult }>(path, { method, body, signal })).data
}

/** Project-readable metadata never includes amounts. */
export function listBonusRounds(team: string, project: string, signal?: AbortSignal) {
  return read<{ items: BonusRoundSummary[]; has_more: boolean }>(
    bonusRoundsPath(team, project),
    signal,
  )
}
export function getBonusRound(
  team: string,
  project: string,
  roundID: string,
  signal?: AbortSignal,
) {
  return read<BonusRoundDetail>(bonusRoundsPath(team, project, roundID), signal)
}
export function getBonusProjectWorkspace(team: string, project: string, signal?: AbortSignal) {
  return read<BonusProjectWorkspace>(`${projectPath(team, project)}/bonus-workspace`, signal)
}
export function getBonusWorkspace(
  team: string,
  project: string,
  roundID: string,
  signal?: AbortSignal,
) {
  return read<BonusWorkspace>(`${bonusRoundsPath(team, project, roundID)}/workspace`, signal)
}
export function createBonusRound(
  team: string,
  project: string,
  body: CreateBonusRound,
  signal?: AbortSignal,
) {
  return write(bonusRoundsPath(team, project), body, signal)
}
export function saveBonusPlan(
  team: string,
  project: string,
  roundID: string,
  body: BonusPlanInput & BonusCommand,
  signal?: AbortSignal,
) {
  return write(`${bonusRoundsPath(team, project, roundID)}/plan`, body, signal, 'PUT')
}
export function transitionBonusRound(
  team: string,
  project: string,
  roundID: string,
  action: BonusAction,
  body: BonusCommand,
  signal?: AbortSignal,
) {
  return write(`${bonusRoundsPath(team, project, roundID)}/${action}`, body, signal)
}
export function recordBonusPayment(
  team: string,
  project: string,
  roundID: string,
  body: BonusPaymentInput & BonusCommand,
  signal?: AbortSignal,
) {
  return write(`${bonusRoundsPath(team, project, roundID)}/payments`, body, signal)
}
export function changeBonusGrant(
  team: string,
  project: string,
  roundID: string,
  body: BonusGrantInput & BonusCommand,
  signal?: AbortSignal,
) {
  return write(`${bonusRoundsPath(team, project, roundID)}/grants`, body, signal)
}
export function listMyBonusResults(team: string, signal?: AbortSignal) {
  return read<{ items: BonusResult[]; has_more: boolean }>(
    `/api/v1/teams/${encodeURIComponent(team)}/bonus-results/mine`,
    signal,
  )
}
