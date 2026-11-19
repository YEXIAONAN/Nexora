import { afterEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from './client'
import * as api from './contributions'

afterEach(() => vi.unstubAllGlobals())

const team = 'team/a'
const project = 'project/b'
const caseID = 'case/c'
const projectURL = '/api/v1/teams/team%2Fa/projects/project%2Fb'
const caseURL = `${projectURL}/contributions/case%2Fc`
const version = {
  expected_version: 7,
  client_request_id: '9c4c99c6-6e5e-4d61-9c7b-523e9f832ea4',
  reason: '人工审核证据',
  case_version_id: 'acb924bf-0065-4860-b359-f8589dac5dc3',
}
const create = {
  task_id: 'task',
  review_id: 'accepted-review',
  reason: version.reason,
  client_request_id: version.client_request_id,
}
const appraiser = {
  user_id: 'user/d',
  expected_version: 0,
  client_request_id: version.client_request_id,
  reason: version.reason,
}
const revoke = { ...appraiser, expected_version: 3 }
const draft = { ...version, summary: '真实成果', evidence_snapshot: { note: '人工整理' } }
const share = {
  ...version,
  share_bps: 3333,
  basis: '贡献依据',
  statement: '编辑者整理的声明',
  evidence_refs: [{ kind: 'objective_fact' as const, source: '审核', reference: 'review' }],
}
const dispute = { ...version, confirmation_id: 'confirmation' }
const resolution = { ...version, decision: 'overturn' as const }
const conflict = { ...version, user_id: 'user/d', kind: 'direct_interest', details: '直接利益' }
const notice = { ...version, confirmation_id: 'confirmation', recipient_id: 'user/d' }
const receipt = {
  expected_version: version.expected_version,
  client_request_id: version.client_request_id,
  case_version_id: version.case_version_id,
  confirmation_id: 'confirmation',
  content_digest: 'a'.repeat(64),
  received: true as const,
}
const intake = {
  ...version,
  team_id: team,
  project_id: project,
  case_id: caseID,
  confirmation_id: 'confirmation',
}

interface Endpoint {
  name: string
  path: string
  method?: string
  body?: unknown
  call: (signal: AbortSignal) => Promise<unknown>
}

const endpoints: Endpoint[] = [
  {
    name: 'appraiser list',
    path: `${projectURL}/contribution-appraisers`,
    call: (s) => api.listContributionAppraisers(team, project, s),
  },
  {
    name: 'appraiser grant',
    path: `${projectURL}/contribution-appraisers`,
    method: 'POST',
    body: appraiser,
    call: (s) => api.grantContributionAppraiser(team, project, appraiser, s),
  },
  {
    name: 'appraiser revoke',
    path: `${projectURL}/contribution-appraisers/user%2Fd`,
    method: 'DELETE',
    body: revoke,
    call: (s) => api.revokeContributionAppraiser(team, project, 'user/d', revoke, s),
  },
  {
    name: 'eligibility',
    path: `${projectURL}/contributions/eligibility`,
    call: (s) => api.getContributionEligibility(team, project, s),
  },
  {
    name: 'appraisable pending',
    path: `${projectURL}/contributions/pending`,
    call: (s) => api.listMyAppraisableContributions(team, project, s),
  },
  {
    name: 'case list',
    path: `${projectURL}/contributions`,
    call: (s) => api.listContributions(team, project, s),
  },
  {
    name: 'accepted-review case creation',
    path: `${projectURL}/contributions`,
    method: 'POST',
    body: create,
    call: (s) => api.createContribution(team, project, create, s),
  },
  {
    name: 'detail',
    path: caseURL,
    call: (s) => api.getContribution(team, project, caseID, s),
  },
  {
    name: 'versions',
    path: `${caseURL}/versions`,
    call: (s) => api.listContributionVersions(team, project, caseID, s),
  },
  {
    name: 'history',
    path: `${caseURL}/history`,
    call: (s) => api.getContributionHistory(team, project, caseID, s),
  },
  {
    name: 'evidence',
    path: `${caseURL}/evidence`,
    call: (s) => api.getContributionEvidence(team, project, caseID, s),
  },
  {
    name: 'draft save',
    path: `${caseURL}/draft`,
    method: 'PATCH',
    body: draft,
    call: (s) => api.saveContributionDraft(team, project, caseID, draft, s),
  },
  {
    name: 'integer share save',
    path: `${caseURL}/shares/user%2Fd`,
    method: 'PUT',
    body: share,
    call: (s) => api.setContributionShare(team, project, caseID, 'user/d', share, s),
  },
  {
    name: 'draft share delete',
    path: `${caseURL}/shares/user%2Fd`,
    method: 'DELETE',
    body: version,
    call: (s) => api.removeContributionShare(team, project, caseID, 'user/d', version, s),
  },
  {
    name: 'submit',
    path: `${caseURL}/submit`,
    method: 'POST',
    body: version,
    call: (s) => api.submitContribution(team, project, caseID, version, s),
  },
  {
    name: 'withdraw',
    path: `${caseURL}/withdraw`,
    method: 'POST',
    body: version,
    call: (s) => api.withdrawContribution(team, project, caseID, version, s),
  },
  {
    name: 'abandon',
    path: `${caseURL}/abandon`,
    method: 'POST',
    body: version,
    call: (s) => api.abandonContribution(team, project, caseID, version, s),
  },
  {
    name: 'independent confirmation',
    path: `${caseURL}/confirm`,
    method: 'POST',
    body: version,
    call: (s) => api.confirmContribution(team, project, caseID, version, s),
  },
  {
    name: 'revision creation',
    path: `${caseURL}/revisions`,
    method: 'POST',
    body: version,
    call: (s) => api.revisionsContribution(team, project, caseID, version, s),
  },
  {
    name: 'dispute list',
    path: `${caseURL}/disputes`,
    call: (s) => api.listContributionDisputes(team, project, caseID, s),
  },
  {
    name: 'raise dispute',
    path: `${caseURL}/disputes`,
    method: 'POST',
    body: dispute,
    call: (s) => api.raiseContributionDispute(team, project, caseID, dispute, s),
  },
  {
    name: 'independent resolution',
    path: `${caseURL}/disputes/dispute%2Fe/resolutions`,
    method: 'POST',
    body: resolution,
    call: (s) => api.resolveContributionDispute(team, project, caseID, 'dispute/e', resolution, s),
  },
  {
    name: 'record conflict',
    path: `${caseURL}/conflicts`,
    method: 'POST',
    body: conflict,
    call: (s) => api.recordContributionConflict(team, project, caseID, conflict, s),
  },
  {
    name: 'notice list',
    path: `${caseURL}/notices`,
    call: (s) => api.listContributionNotices(team, project, caseID, s),
  },
  {
    name: 'prepare notice',
    path: `${caseURL}/notices`,
    method: 'POST',
    body: notice,
    call: (s) => api.prepareContributionNotice(team, project, caseID, notice, s),
  },
  {
    name: 'explicit authenticated receipt',
    path: `${caseURL}/notices/notice%2Ff/receipt`,
    method: 'POST',
    body: receipt,
    call: (s) => api.acknowledgeContributionReceipt(team, project, caseID, 'notice/f', receipt, s),
  },
  {
    name: 'my results (GET never acknowledges)',
    path: `${projectURL}/contribution-results/mine`,
    call: (s) => api.listMyContributionResults(team, project, s),
  },
  {
    name: 'privacy-preserving intake',
    path: '/api/v1/contribution-dispute-requests',
    method: 'POST',
    body: intake,
    call: (s) => api.receiveContributionDisputeRequest(intake, s),
  },
  {
    name: 'scoped verified identity evidence',
    path: `${caseURL}/dispute-requests/request%2Fg`,
    call: (s) => api.getVerifiedContributionDisputeRequest(team, project, caseID, 'request/g', s),
  },
]

describe('contribution OpenAPI operations', () => {
  it.each(endpoints)(
    '$name preserves method, scope, version and request identity',
    async (endpoint) => {
      const data = { authoritative: true }
      const fetchMock = vi.fn(async (_input: RequestInfo | URL, _options?: RequestInit) => ({
        ok: true,
        status: endpoint.name === 'privacy-preserving intake' ? 202 : 200,
        json: async () => ({ data }),
      }))
      vi.stubGlobal('fetch', fetchMock)
      const controller = new AbortController()
      expect(await endpoint.call(controller.signal)).toEqual(data)
      expect(fetchMock).toHaveBeenCalledTimes(1)
      const [path, options] = fetchMock.mock.calls[0]!
      expect(path).toBe(endpoint.path)
      expect(options?.method).toBe(endpoint.method ?? 'GET')
      expect(options?.body).toBe(endpoint.body ? JSON.stringify(endpoint.body) : undefined)
      expect(options?.credentials).toBe('same-origin')
      expect(options?.signal).toBeInstanceOf(AbortSignal)
      expect((options?.headers as Record<string, string>)['X-Nexora-CSRF']).toBe(
        endpoint.method ? '1' : undefined,
      )
    },
  )

  it.each([401, 403, 404, 409, 422, 500])(
    'propagates HTTP %s for the caller to handle',
    async (status) => {
      vi.stubGlobal(
        'fetch',
        vi.fn(async () => ({
          ok: false,
          status,
          json: async () => ({ error: { code: 'contribution_failure', message: '服务端拒绝' } }),
        })),
      )
      await expect(api.getContribution(team, project, caseID)).rejects.toMatchObject({ status })
    },
  )

  it('propagates cancellation through the existing API client', async () => {
    const caller = new AbortController()
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (_input: RequestInfo | URL, options?: RequestInit) =>
          new Promise((_resolve, reject) =>
            options?.signal?.addEventListener('abort', () =>
              reject(new DOMException('canceled', 'AbortError')),
            ),
          ),
      ),
    )
    const result = api.listContributions(team, project, caller.signal)
    caller.abort()
    await expect(result).rejects.toBeInstanceOf(ApiError)
  })
})
