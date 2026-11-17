import { requestJSON } from './client'
import { projectPath } from './projects'
import type { components } from './schema'
type S = components['schemas']
export type SubmissionVersion = S['SubmissionVersion']
export type EvidenceReference = S['EvidenceReference']
export type Attachment = S['Attachment']
export type Reviewer = S['ProjectReviewer']
export type PendingReview = S['PendingReview']
export const deliveryPath = (team: string, project: string, task: string) =>
  `${projectPath(team, project)}/tasks/${encodeURIComponent(task)}`
export async function listVersions(team: string, project: string, task: string) {
  return (
    await requestJSON<S['SubmissionVersionsEnvelope']>(
      `${deliveryPath(team, project, task)}/submissions`,
    )
  ).data
}
export async function submitVersion(
  team: string,
  project: string,
  task: string,
  body: S['SubmitVersionRequest'],
) {
  return (
    await requestJSON<S['SubmissionVersionEnvelope']>(
      `${deliveryPath(team, project, task)}/submissions`,
      { method: 'POST', body },
    )
  ).data
}
export async function reviewVersion(
  team: string,
  project: string,
  task: string,
  version: string,
  body: S['ReviewVersionRequest'],
) {
  return (
    await requestJSON<S['SubmissionReviewEnvelope']>(
      `${deliveryPath(team, project, task)}/submissions/${encodeURIComponent(version)}/reviews`,
      { method: 'POST', body },
    )
  ).data
}
export async function uploadAttachment(
  team: string,
  project: string,
  task: string,
  file: File,
  key: string,
) {
  const body = new FormData()
  body.append('file', file)
  return (
    await requestJSON<S['AttachmentEnvelope']>(`${deliveryPath(team, project, task)}/attachments`, {
      method: 'POST',
      body,
      headers: { 'X-Nexora-Upload-Id': key },
      timeoutMs: 30000,
    })
  ).data
}
export const attachmentURL = (
  team: string,
  project: string,
  task: string,
  version: string,
  attachment: string,
) =>
  `${import.meta.env.VITE_API_BASE_URL ?? ''}${deliveryPath(team, project, task)}/submissions/${encodeURIComponent(version)}/attachments/${encodeURIComponent(attachment)}`
export async function listReviewers(team: string, project: string) {
  return (
    await requestJSON<S['ProjectReviewersEnvelope']>(`${projectPath(team, project)}/reviewers`)
  ).data
}
export async function grantReviewer(
  team: string,
  project: string,
  user_id: string,
  reason: string,
) {
  return (
    await requestJSON<S['ProjectReviewersEnvelope']>(`${projectPath(team, project)}/reviewers`, {
      method: 'POST',
      body: { user_id, reason },
    })
  ).data
}
export async function revokeReviewer(
  team: string,
  project: string,
  reviewer: Reviewer,
  reason: string,
) {
  return (
    await requestJSON<S['ProjectReviewersEnvelope']>(
      `${projectPath(team, project)}/reviewers/${encodeURIComponent(reviewer.user_id)}`,
      { method: 'DELETE', body: { expected_version: reviewer.version, reason } },
    )
  ).data
}
export async function pendingReviews(team: string) {
  return (
    await requestJSON<S['PendingReviewsEnvelope']>(
      `/api/v1/teams/${encodeURIComponent(team)}/reviews/pending`,
    )
  ).data
}
