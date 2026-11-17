import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useTeamsStore } from './teams'

const teamA = {
  id: '11111111-1111-4111-8111-111111111111',
  name: 'Team A',
  description: '',
  created_by: 'u1',
  created_at: '2026-11-17T00:00:00Z',
  updated_at: '2026-11-17T00:00:00Z',
  my_role: 'owner' as const,
}
const teamB = {
  id: '22222222-2222-4222-8222-222222222222',
  name: 'Team B',
  description: '',
  created_by: 'u2',
  created_at: '2026-11-17T00:00:00Z',
  updated_at: '2026-11-17T00:00:00Z',
  my_role: 'member' as const,
}

function jsonResponse(payload: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => payload,
  } as Response
}

function stubFetch(handler: (input: string, init?: RequestInit) => Response | Promise<Response>) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) =>
      Promise.resolve(handler(String(input), init)),
    ),
  )
}

describe('teams store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    window.localStorage.clear()
  })

  afterEach(() => vi.unstubAllGlobals())

  it('loads my teams and selects the first when no preference exists', async () => {
    stubFetch(() => jsonResponse({ data: [teamA, teamB] }))
    const teams = useTeamsStore()
    await teams.loadTeams('u1')
    expect(teams.teams).toHaveLength(2)
    expect(teams.currentTeamId).toBe(teamA.id)
    expect(teams.currentTeam?.name).toBe('Team A')
    expect(window.localStorage.getItem('nexora.currentTeam')).toBe(teamA.id)
  })

  it('honours a persisted preference that is still a member team', async () => {
    window.localStorage.setItem('nexora.currentTeam', teamB.id)
    stubFetch(() => jsonResponse({ data: [teamA, teamB] }))
    const teams = useTeamsStore()
    await teams.loadTeams('u1')
    expect(teams.currentTeamId).toBe(teamB.id)
  })

  it('falls back when the persisted team is no longer accessible', async () => {
    window.localStorage.setItem('nexora.currentTeam', '99999999-9999-4999-8999-999999999999')
    stubFetch(() => jsonResponse({ data: [teamA] }))
    const teams = useTeamsStore()
    await teams.loadTeams('u1')
    expect(teams.currentTeamId).toBe(teamA.id)
    expect(window.localStorage.getItem('nexora.currentTeam')).toBe(teamA.id)
  })

  it('selectTeam ignores teams the user is not a member of', async () => {
    stubFetch(() => jsonResponse({ data: [teamA] }))
    const teams = useTeamsStore()
    await teams.loadTeams('u1')
    teams.selectTeam(teamB.id)
    expect(teams.currentTeamId).toBe(teamA.id)
    teams.selectTeam(teamA.id)
    expect(teams.currentTeamId).toBe(teamA.id)
  })

  it('createTeam appends the real API result and selects it', async () => {
    stubFetch((_input, init) => {
      if (init?.method === 'POST') {
        expect((init.headers as Record<string, string>)['X-Nexora-CSRF']).toBe('1')
        return jsonResponse({ data: { ...teamB, my_role: 'owner' } }, 201)
      }
      return jsonResponse({ data: [teamA] })
    })
    const teams = useTeamsStore()
    await teams.loadTeams('u1')
    const created = await teams.createTeam('Team B', 'desc')
    expect(created.id).toBe(teamB.id)
    expect(teams.teams.map((t) => t.id)).toEqual([teamA.id, teamB.id])
    expect(teams.currentTeamId).toBe(teamB.id)
  })

  it('surfaces API errors and clears stale state', async () => {
    stubFetch(() => jsonResponse({ error: { code: 'unauthorized', message: '会话已失效' } }, 401))
    const teams = useTeamsStore()
    await teams.loadTeams('u1')
    expect(teams.error).toBe('会话已失效')
    expect(teams.teams).toHaveLength(0)
    expect(teams.currentTeamId).toBeNull()
  })

  it('reset clears selection and the persisted preference', async () => {
    stubFetch(() => jsonResponse({ data: [teamA] }))
    const teams = useTeamsStore()
    await teams.loadTeams('u1')
    teams.reset()
    expect(teams.teams).toHaveLength(0)
    expect(teams.currentTeamId).toBeNull()
    expect(window.localStorage.getItem('nexora.currentTeam')).toBeNull()
  })
})
