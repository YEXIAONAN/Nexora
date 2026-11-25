import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/client'
import type { User } from '@/api/auth'
import type { Team, TeamInvitation, TeamMember, TeamPermissions } from '@/api/teams'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
import { useUiStore } from '@/stores/ui'

import TeamMembersView from './TeamMembersView.vue'

// Every API call returns a deferred promise the test resolves by hand, so the
// completion order is fully controlled — no random delays or sleeps.
const hoisted = vi.hoisted(() => {
  interface Deferred {
    promise: Promise<unknown>
    resolve: (value: unknown) => void
    reject: (reason: unknown) => void
  }
  const calls: Record<string, Deferred[]> = {}
  function enqueue(key: string): Promise<unknown> {
    let resolve!: (value: unknown) => void
    let reject!: (reason: unknown) => void
    const promise = new Promise<unknown>((res, rej) => {
      resolve = res
      reject = rej
    })
    const list = calls[key] ?? (calls[key] = [])
    list.push({ promise, resolve, reject })
    return promise
  }
  const pushMock = vi.fn()
  const routeHolder: { route: { params: { teamId: string } } | null } = { route: null }
  return { calls, enqueue, pushMock, routeHolder }
})

vi.mock('vue-router', async () => {
  const { defineComponent, h, reactive } = await import('vue')
  const route = reactive({ params: { teamId: '' } })
  hoisted.routeHolder.route = route
  return {
    useRoute: () => route,
    useRouter: () => ({ push: hoisted.pushMock }),
    RouterLink: defineComponent({
      props: { to: { type: String, required: true } },
      setup(props, { slots }) {
        return () => h('a', { href: props.to }, slots.default?.())
      },
    }),
  }
})

vi.mock('@/api/teams', () => ({
  getTeam: (id: string) => hoisted.enqueue(`getTeam:${id}`),
  getTeamPermissions: (id: string) => hoisted.enqueue(`getTeamPermissions:${id}`),
  listTeamMembers: (id: string) => hoisted.enqueue(`listTeamMembers:${id}`),
  listTeamInvitations: (id: string) => hoisted.enqueue(`listTeamInvitations:${id}`),
  changeMemberRole: (id: string, userId: string) =>
    hoisted.enqueue(`changeMemberRole:${id}:${userId}`),
  removeTeamMember: (id: string, userId: string) =>
    hoisted.enqueue(`removeTeamMember:${id}:${userId}`),
  createInvitation: (id: string, username: string) =>
    hoisted.enqueue(`createInvitation:${id}:${username}`),
  cancelInvitation: (id: string, invitationId: string) =>
    hoisted.enqueue(`cancelInvitation:${id}:${invitationId}`),
  listMyTeams: () => hoisted.enqueue('listMyTeams'),
  createTeam: (name: string) => hoisted.enqueue(`createTeam:${name}`),
}))

const me: User = {
  id: 'u-me',
  username: 'waiting',
  display_name: 'Waiting',
  system_root: false,
  platform_admin: false,
}

function teamFixture(id: string, name: string): Team {
  return {
    id,
    name,
    description: `${name} 描述`,
    created_by: 'u-me',
    created_at: '2026-11-17T00:00:00Z',
    updated_at: '2026-11-17T00:00:00Z',
    my_role: 'owner',
  }
}

function permsFixture(id: string, over: Partial<TeamPermissions> = {}): TeamPermissions {
  return {
    team_id: id,
    role: 'owner',
    can_invite: true,
    can_manage_invitations: true,
    can_remove_members: true,
    can_change_roles: true,
    can_leave: true,
    ...over,
  }
}

function memberFixture(userId: string, username: string, role: TeamMember['role']): TeamMember {
  return {
    user_id: userId,
    username,
    display_name: username,
    role,
    joined_at: '2026-11-17T00:00:00Z',
  }
}

const teamA = teamFixture('team-a', 'Team A')
const teamB = teamFixture('team-b', 'Team B')
const permsA = permsFixture('team-a')
const permsB = permsFixture('team-b')
const meMember = memberFixture('u-me', 'waiting', 'owner')
const shared = memberFixture('u-x', 'shared', 'member')
const memberA = memberFixture('u-a', 'membera', 'member')
const memberB = memberFixture('u-b', 'memberb', 'member')

interface LoadData {
  team: Team
  permissions: TeamPermissions
  members: TeamMember[]
  invitations?: TeamInvitation[]
}

type Wrapper = ReturnType<typeof mount>

function memberIds(wrapper: Wrapper): (string | undefined)[] {
  return wrapper.findAll('li[data-user-id]').map((li) => li.attributes('data-user-id'))
}

function resolveCore(id: string, index: number, data: LoadData): void {
  hoisted.calls[`getTeam:${id}`][index].resolve(data.team)
  hoisted.calls[`getTeamPermissions:${id}`][index].resolve(data.permissions)
  hoisted.calls[`listTeamMembers:${id}`][index].resolve(data.members)
}

async function settle(id: string, index: number, data: LoadData, invIndex = 0): Promise<void> {
  resolveCore(id, index, data)
  await flushPromises()
  if (data.permissions.can_manage_invitations) {
    hoisted.calls[`listTeamInvitations:${id}`][invIndex].resolve(data.invitations ?? [])
    await flushPromises()
  }
}

async function navigate(id: string): Promise<void> {
  hoisted.routeHolder.route!.params.teamId = id
  await flushPromises()
}

async function mountView(initial: string) {
  hoisted.routeHolder.route!.params.teamId = initial
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = me
  const teamsStore = useTeamsStore()
  teamsStore.teams = [teamA, teamB]
  const wrapper = mount(TeamMembersView, {
    global: { plugins: [pinia], stubs: { AppIcon: true } },
  })
  await flushPromises()
  return { wrapper, teamsStore }
}

const loadA: LoadData = { team: teamA, permissions: permsA, members: [memberA], invitations: [] }
const loadB: LoadData = { team: teamB, permissions: permsB, members: [memberB], invitations: [] }

beforeEach(() => {
  window.localStorage.clear()
  hoisted.pushMock.mockClear()
  for (const key of Object.keys(hoisted.calls)) delete hoisted.calls[key]
})

describe('TeamMembersView team-switch races', () => {
  it('ignores a delayed team A response after switching to team B', async () => {
    const { wrapper, teamsStore } = await mountView('team-a')
    await navigate('team-b')
    await settle('team-b', 0, loadB)
    expect(wrapper.text()).toContain('Team B')

    // Team A's request completes last.
    resolveCore('team-a', 0, loadA)
    await flushPromises()

    expect(wrapper.text()).toContain('Team B')
    expect(wrapper.text()).not.toContain('Team A')
    expect(memberIds(wrapper)).toEqual(['u-b'])
    expect(teamsStore.currentTeamId).toBe('team-b')
    expect(window.localStorage.getItem('nexora.currentTeam')).toBe('team-b')
    wrapper.unmount()
  })

  it('does not surface a stale team A error after team B loads', async () => {
    const { wrapper } = await mountView('team-a')
    await navigate('team-b')
    await settle('team-b', 0, loadB)

    hoisted.calls['getTeam:team-a'][0].reject(new ApiError('团队不存在', 404, 'not_found'))
    await flushPromises()

    expect(wrapper.text()).toContain('Team B')
    expect(wrapper.text()).not.toContain('团队不存在')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(memberIds(wrapper)).toEqual(['u-b'])
    wrapper.unmount()
  })

  it('keeps the latest team A data after rapid A→B→A navigation', async () => {
    const { wrapper } = await mountView('team-a') // generation 1 (A)
    await navigate('team-b') // generation 2 (B)
    await navigate('team-a') // generation 3 (A)

    const newest = memberFixture('u-a-new', 'newa', 'member')
    await settle('team-a', 1, { team: teamA, permissions: permsA, members: [newest] }, 0)
    expect(memberIds(wrapper)).toEqual(['u-a-new'])

    // Generation 2 (B) and generation 1 (A) both complete afterwards.
    resolveCore('team-b', 0, loadB)
    await flushPromises()
    resolveCore('team-a', 0, {
      team: teamA,
      permissions: permsA,
      members: [memberFixture('u-a-old', 'olda', 'member')],
    })
    await flushPromises()

    expect(memberIds(wrapper)).toEqual(['u-a-new'])
    expect(wrapper.text()).toContain('Team A')
    expect(window.localStorage.getItem('nexora.currentTeam')).toBe('team-a')
    wrapper.unmount()
  })

  it('binds a removal to the confirmed team and never targets the new team', async () => {
    const { wrapper } = await mountView('team-a')
    await settle('team-a', 0, { team: teamA, permissions: permsA, members: [shared, meMember] })

    await wrapper.find('li[data-user-id="u-x"] button').trigger('click')
    await flushPromises()
    useUiStore().resolveConfirm(true)
    await flushPromises()
    expect(hoisted.calls['removeTeamMember:team-a:u-x']).toHaveLength(1)

    // Route switches to B (which also contains u-x) before the removal resolves.
    await navigate('team-b')
    await settle('team-b', 0, { team: teamB, permissions: permsB, members: [shared, meMember] })

    hoisted.calls['removeTeamMember:team-a:u-x'][0].resolve({ data: { removed: true } })
    await flushPromises()

    expect(hoisted.calls['removeTeamMember:team-b:u-x']).toBeUndefined()
    expect(wrapper.text()).toContain('Team B')
    expect(wrapper.text()).not.toContain('已移除成员')
    wrapper.unmount()
  })

  it('clears member and invitation controls while the new team is loading', async () => {
    const { wrapper } = await mountView('team-a')
    await settle('team-a', 0, { team: teamA, permissions: permsA, members: [shared, meMember] })
    expect(wrapper.find('li[data-user-id="u-x"] button').exists()).toBe(true)

    await navigate('team-b') // B in flight, old state cleared

    expect(memberIds(wrapper)).toEqual([])
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.find('li[data-user-id="u-x"] button').exists()).toBe(false)

    await settle('team-b', 0, { team: teamB, permissions: permsB, members: [memberB] })
    expect(memberIds(wrapper)).toEqual(['u-b'])
    wrapper.unmount()
  })

  it('exposes no member actions when the team fails to load', async () => {
    const { wrapper } = await mountView('team-a')
    hoisted.calls['getTeam:team-a'][0].reject(new ApiError('团队不存在', 404, 'not_found'))
    await flushPromises()

    expect(wrapper.text()).toContain('团队不存在')
    expect(memberIds(wrapper)).toEqual([])
    expect(wrapper.find('form').exists()).toBe(false)
    wrapper.unmount()
  })

  it('exposes no management controls when the confirmed team grants no permissions', async () => {
    const { wrapper } = await mountView('team-a')
    const readOnly = permsFixture('team-a', {
      role: 'member',
      can_invite: false,
      can_manage_invitations: false,
      can_remove_members: false,
      can_change_roles: false,
    })
    await settle('team-a', 0, { team: teamA, permissions: readOnly, members: [shared, meMember] })

    expect(wrapper.find('li[data-user-id="u-x"] select').exists()).toBe(false)
    expect(wrapper.find('li[data-user-id="u-x"] button').exists()).toBe(false)
    expect(wrapper.find('form').exists()).toBe(false)
    wrapper.unmount()
  })

  it('applies a role change on the confirmed team and reloads it', async () => {
    const { wrapper } = await mountView('team-a')
    await settle('team-a', 0, { team: teamA, permissions: permsA, members: [shared, meMember] })

    await wrapper.find('li[data-user-id="u-x"] select').setValue('admin')
    await flushPromises()
    hoisted.calls['changeMemberRole:team-a:u-x'][0].resolve({ data: { role: 'admin' } })
    await flushPromises()

    // The mutation reloads team A (generation 2).
    await settle(
      'team-a',
      1,
      { team: teamA, permissions: permsA, members: [memberFixture('u-x', 'shared', 'admin')] },
      1,
    )
    expect(wrapper.text()).toContain('已将 shared 的角色变更为管理员')
    expect(hoisted.calls['changeMemberRole:team-b:u-x']).toBeUndefined()
    wrapper.unmount()
  })

  it('does not apply a stale invitation result to the new team', async () => {
    const { wrapper } = await mountView('team-a')
    await settle('team-a', 0, { team: teamA, permissions: permsA, members: [meMember] })

    await wrapper.find('#invite-username').setValue('newuser')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(hoisted.calls['createInvitation:team-a:newuser']).toHaveLength(1)

    await navigate('team-b')
    await settle('team-b', 0, { team: teamB, permissions: permsB, members: [meMember] })

    hoisted.calls['createInvitation:team-a:newuser'][0].resolve({ data: {} })
    await flushPromises()

    expect(hoisted.calls['createInvitation:team-b:newuser']).toBeUndefined()
    expect(hoisted.calls['listTeamInvitations:team-b']).toBeDefined()
    expect(wrapper.text()).toContain('Team B')
    expect(wrapper.text()).not.toContain('已向 newuser 发出邀请')
    wrapper.unmount()
  })
})
