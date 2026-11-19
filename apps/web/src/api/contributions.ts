import { requestJSON } from './client'
import { projectPath } from './projects'
import type { components } from './schema'

type S = components['schemas']
export type ContributionCase = S['ContributionCase']
export type ContributionCaseDetail = S['ContributionCaseDetail']
export type ContributionVersion = S['ContributionVersion']
export type ContributionShare = S['ContributionShare']
export type ContributionConfirmation = S['ContributionConfirmation']
export type ContributionPermissions = S['ContributionPermissions']
export type ContributionEligibility = S['ContributionEligibility']
export type ContributionAppraiser = S['ContributionAppraiser']
export type ContributionEvidence = S['ContributionEvidence']
export type ContributionEvidenceReference = S['ContributionEvidenceReference']
export type ContributionHistory = S['ContributionHistory']
export type ContributionDispute = S['ContributionDispute']
export type ContributionResolution = S['ContributionResolution']
export type ContributionNotice = S['ContributionNotice']
export type ContributionReceipt = S['ContributionReceipt']
export type ContributionResult = S['ContributionResult']
export type ContributionCommandResult = S['ContributionCommandResult']
export type ContributionCommand = S['ContributionCommand']
export type ContributionVersionCommand = S['ContributionVersionCommand']
export type ContributionCreateRequest = S['ContributionCreateRequest']
export type ContributionAppraiserRequest = S['ContributionAppraiserRequest']
export type ContributionRevokeRequest = S['ContributionRevokeRequest']
export type ContributionDraftRequest = S['ContributionDraftRequest']
export type ContributionShareRequest = S['ContributionShareRequest']
export type ContributionDisputeRequest = S['ContributionDisputeRequest']
export type ContributionResolutionRequest = S['ContributionResolutionRequest']
export type ContributionConflictRequest = S['ContributionConflictRequest']
export type ContributionNoticeRequest = S['ContributionNoticeRequest']
export type ContributionReceiptRequest = S['ContributionReceiptRequest']
export type ContributionIdentityRequest = S['ContributionIdentityRequest']
export type ContributionIdentityEvidence = S['ContributionIdentityEvidence']
export type ContributionIntake = S['ContributionIntake']

export const contributionPath = (team: string, project: string, caseID?: string) =>
  `${projectPath(team, project)}/contributions${caseID ? `/${encodeURIComponent(caseID)}` : ''}`

async function read<T>(path: string, signal?: AbortSignal): Promise<T> {
  return (await requestJSON<{ data: T }>(path, { signal })).data
}

async function command(
  path: string,
  method: 'POST' | 'PATCH' | 'PUT' | 'DELETE',
  body: unknown,
  signal?: AbortSignal,
): Promise<ContributionCommandResult> {
  return (await requestJSON<S['ContributionCommandResultEnvelope']>(path, { method, body, signal }))
    .data
}

export function listContributionAppraisers(team: string, project: string, signal?: AbortSignal) {
  return read<ContributionAppraiser[]>(
    `${projectPath(team, project)}/contribution-appraisers`,
    signal,
  )
}

export function grantContributionAppraiser(
  team: string,
  project: string,
  body: S['ContributionAppraiserRequest'],
  signal?: AbortSignal,
) {
  return command(`${projectPath(team, project)}/contribution-appraisers`, 'POST', body, signal)
}

export function revokeContributionAppraiser(
  team: string,
  project: string,
  userID: string,
  body: S['ContributionRevokeRequest'],
  signal?: AbortSignal,
) {
  return command(
    `${projectPath(team, project)}/contribution-appraisers/${encodeURIComponent(userID)}`,
    'DELETE',
    body,
    signal,
  )
}

export function getContributionEligibility(team: string, project: string, signal?: AbortSignal) {
  return read<ContributionEligibility>(`${contributionPath(team, project)}/eligibility`, signal)
}

export function listMyAppraisableContributions(
  team: string,
  project: string,
  signal?: AbortSignal,
) {
  return read<ContributionCase[]>(`${contributionPath(team, project)}/pending`, signal)
}

export function listContributions(team: string, project: string, signal?: AbortSignal) {
  return read<ContributionCase[]>(contributionPath(team, project), signal)
}

export function createContribution(
  team: string,
  project: string,
  body: S['ContributionCreateRequest'],
  signal?: AbortSignal,
) {
  return command(contributionPath(team, project), 'POST', body, signal)
}

export function getContribution(
  team: string,
  project: string,
  caseID: string,
  signal?: AbortSignal,
) {
  return read<ContributionCaseDetail>(contributionPath(team, project, caseID), signal)
}

export function listContributionVersions(
  team: string,
  project: string,
  caseID: string,
  signal?: AbortSignal,
) {
  return read<ContributionVersion[]>(`${contributionPath(team, project, caseID)}/versions`, signal)
}

export function getContributionHistory(
  team: string,
  project: string,
  caseID: string,
  signal?: AbortSignal,
) {
  return read<ContributionHistory>(`${contributionPath(team, project, caseID)}/history`, signal)
}

export function getContributionEvidence(
  team: string,
  project: string,
  caseID: string,
  signal?: AbortSignal,
) {
  return read<ContributionEvidence>(`${contributionPath(team, project, caseID)}/evidence`, signal)
}

export function saveContributionDraft(
  team: string,
  project: string,
  caseID: string,
  body: S['ContributionDraftRequest'],
  signal?: AbortSignal,
) {
  return command(`${contributionPath(team, project, caseID)}/draft`, 'PATCH', body, signal)
}

export function setContributionShare(
  team: string,
  project: string,
  caseID: string,
  userID: string,
  body: S['ContributionShareRequest'],
  signal?: AbortSignal,
) {
  return command(
    `${contributionPath(team, project, caseID)}/shares/${encodeURIComponent(userID)}`,
    'PUT',
    body,
    signal,
  )
}

export function removeContributionShare(
  team: string,
  project: string,
  caseID: string,
  userID: string,
  body: S['ContributionVersionCommand'],
  signal?: AbortSignal,
) {
  return command(
    `${contributionPath(team, project, caseID)}/shares/${encodeURIComponent(userID)}`,
    'DELETE',
    body,
    signal,
  )
}

export function submitContribution(
  team: string,
  project: string,
  caseID: string,
  body: S['ContributionVersionCommand'],
  signal?: AbortSignal,
) {
  return command(`${contributionPath(team, project, caseID)}/submit`, 'POST', body, signal)
}

export function withdrawContribution(
  team: string,
  project: string,
  caseID: string,
  body: S['ContributionVersionCommand'],
  signal?: AbortSignal,
) {
  return command(`${contributionPath(team, project, caseID)}/withdraw`, 'POST', body, signal)
}

export function abandonContribution(
  team: string,
  project: string,
  caseID: string,
  body: S['ContributionVersionCommand'],
  signal?: AbortSignal,
) {
  return command(`${contributionPath(team, project, caseID)}/abandon`, 'POST', body, signal)
}

export function confirmContribution(
  team: string,
  project: string,
  caseID: string,
  body: S['ContributionVersionCommand'],
  signal?: AbortSignal,
) {
  return command(`${contributionPath(team, project, caseID)}/confirm`, 'POST', body, signal)
}

export function revisionsContribution(
  team: string,
  project: string,
  caseID: string,
  body: S['ContributionVersionCommand'],
  signal?: AbortSignal,
) {
  return command(`${contributionPath(team, project, caseID)}/revisions`, 'POST', body, signal)
}

export function listContributionDisputes(
  team: string,
  project: string,
  caseID: string,
  signal?: AbortSignal,
) {
  return read<ContributionDispute[]>(`${contributionPath(team, project, caseID)}/disputes`, signal)
}

export function raiseContributionDispute(
  team: string,
  project: string,
  caseID: string,
  body: S['ContributionDisputeRequest'],
  signal?: AbortSignal,
) {
  return command(`${contributionPath(team, project, caseID)}/disputes`, 'POST', body, signal)
}

export function resolveContributionDispute(
  team: string,
  project: string,
  caseID: string,
  disputeID: string,
  body: S['ContributionResolutionRequest'],
  signal?: AbortSignal,
) {
  return command(
    `${contributionPath(team, project, caseID)}/disputes/${encodeURIComponent(disputeID)}/resolutions`,
    'POST',
    body,
    signal,
  )
}

export function recordContributionConflict(
  team: string,
  project: string,
  caseID: string,
  body: S['ContributionConflictRequest'],
  signal?: AbortSignal,
) {
  return command(`${contributionPath(team, project, caseID)}/conflicts`, 'POST', body, signal)
}

export function listContributionNotices(
  team: string,
  project: string,
  caseID: string,
  signal?: AbortSignal,
) {
  return read<ContributionNotice[]>(`${contributionPath(team, project, caseID)}/notices`, signal)
}

export function prepareContributionNotice(
  team: string,
  project: string,
  caseID: string,
  body: S['ContributionNoticeRequest'],
  signal?: AbortSignal,
) {
  return command(`${contributionPath(team, project, caseID)}/notices`, 'POST', body, signal)
}

export function acknowledgeContributionReceipt(
  team: string,
  project: string,
  caseID: string,
  noticeID: string,
  body: S['ContributionReceiptRequest'],
  signal?: AbortSignal,
) {
  return command(
    `${contributionPath(team, project, caseID)}/notices/${encodeURIComponent(noticeID)}/receipt`,
    'POST',
    body,
    signal,
  )
}

export function listMyContributionResults(team: string, project: string, signal?: AbortSignal) {
  return read<ContributionResult[]>(
    `${projectPath(team, project)}/contribution-results/mine`,
    signal,
  )
}

export async function receiveContributionDisputeRequest(
  body: S['ContributionIdentityRequest'],
  signal?: AbortSignal,
) {
  return (
    await requestJSON<S['ContributionIntakeEnvelope']>('/api/v1/contribution-dispute-requests', {
      method: 'POST',
      body,
      signal,
    })
  ).data
}

export function getVerifiedContributionDisputeRequest(
  team: string,
  project: string,
  caseID: string,
  identityRequestID: string,
  signal?: AbortSignal,
) {
  return read<S['ContributionIdentityEvidence']>(
    `${contributionPath(team, project, caseID)}/dispute-requests/${encodeURIComponent(identityRequestID)}`,
    signal,
  )
}
