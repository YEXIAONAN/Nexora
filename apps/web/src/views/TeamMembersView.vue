<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'

import { ApiError } from '@/api/client'
import {
  cancelInvitation,
  changeMemberRole,
  createInvitation,
  getTeam,
  getTeamPermissions,
  listTeamInvitations,
  listTeamMembers,
  removeTeamMember,
  type InvitationRole,
  type Team,
  type TeamInvitation,
  type TeamMember,
  type TeamPermissions,
  type TeamRole,
} from '@/api/teams'
import AppIcon from '@/components/ui/AppIcon.vue'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
import { formatDateTime, invitationStatusLabel, teamRoleLabel } from '@/utils/format'

const auth = useAuthStore()
const teams = useTeamsStore()
const route = useRoute()
const router = useRouter()

const teamId = computed(() => String(route.params.teamId))
const team = ref<Team | null>(null)
const permissions = ref<TeamPermissions | null>(null)
const members = ref<TeamMember[]>([])
const invitations = ref<TeamInvitation[]>([])
const loading = ref(false)
const loadError = ref<string | null>(null)
const notFound = ref(false)
const actionError = ref<string | null>(null)
const actionMessage = ref<string | null>(null)
const busy = ref<string | null>(null)

// confirmedTeamId is the team whose data is fully loaded and displayed. It is
// set only after every response for that team has been applied, so actions and
// preference writes never bind to a team that is still loading or stale.
const confirmedTeamId = ref<string | null>(null)
let loadGeneration = 0

const inviteUsername = ref('')
const inviteRole = ref<InvitationRole>('member')
const inviteHours = ref('72')

const pendingInvitations = computed(() => invitations.value.filter((i) => i.status === 'pending'))
const historyInvitations = computed(() => invitations.value.filter((i) => i.status !== 'pending'))

// Actions are enabled only when the displayed team is confirmed, matches the
// current route, and nothing is loading or in flight. The instant the route
// changes this turns false, disabling controls bound to the previous team.
const actionsEnabled = computed(
  () =>
    !loading.value &&
    busy.value === null &&
    confirmedTeamId.value !== null &&
    confirmedTeamId.value === teamId.value,
)

function messageOf(reason: unknown): string {
  return reason instanceof ApiError ? reason.message : '无法连接到 Nexora API'
}

function clearTeamState(): void {
  team.value = null
  permissions.value = null
  members.value = []
  invitations.value = []
  confirmedTeamId.value = null
  actionError.value = null
  actionMessage.value = null
}

async function loadAll(): Promise<void> {
  const id = teamId.value
  const generation = ++loadGeneration
  clearTeamState()
  loading.value = true
  loadError.value = null
  notFound.value = false
  const stale = () => generation !== loadGeneration || id !== teamId.value
  try {
    const [detail, caps, memberList] = await Promise.all([
      getTeam(id),
      getTeamPermissions(id),
      listTeamMembers(id),
    ])
    if (stale()) return
    team.value = detail
    permissions.value = caps
    members.value = memberList
    const invites = caps.can_manage_invitations ? await listTeamInvitations(id) : []
    if (stale()) return
    invitations.value = invites
    confirmedTeamId.value = id
    teams.selectTeam(id)
  } catch (reason) {
    if (stale()) return
    if (reason instanceof ApiError && reason.status === 404) notFound.value = true
    else loadError.value = messageOf(reason)
  } finally {
    if (!stale()) loading.value = false
  }
}

onMounted(loadAll)
watch(teamId, (next, previous) => {
  if (next !== previous) void loadAll()
})

function roleOptions(target: TeamMember): TeamRole[] {
  const caps = permissions.value
  if (!caps?.can_change_roles) return []
  if (caps.role === 'owner') return ['owner', 'admin', 'member']
  if (target.role === 'owner') return []
  return ['admin', 'member']
}

function canRemove(target: TeamMember): boolean {
  const caps = permissions.value
  if (!caps || target.user_id === auth.user?.id) return false
  if (!caps.can_remove_members) return false
  return !(caps.role === 'admin' && target.role === 'owner')
}

// runTeamAction binds a mutation to the confirmed team captured at click time
// and re-checks the route before applying any result, so a stale action can
// neither target nor pollute a different team.
async function runTeamAction(key: string, action: (id: string) => Promise<void>): Promise<void> {
  const id = confirmedTeamId.value
  if (!actionsEnabled.value || id === null) return
  busy.value = key
  actionError.value = null
  actionMessage.value = null
  try {
    await action(id)
  } catch (reason) {
    if (id === teamId.value) actionError.value = messageOf(reason)
  } finally {
    if (busy.value === key) busy.value = null
  }
}

async function changeRole(target: TeamMember, role: TeamRole): Promise<void> {
  await runTeamAction(`role:${target.user_id}`, async (id) => {
    await changeMemberRole(id, target.user_id, role)
    if (id !== teamId.value) return
    await loadAll()
    if (id === teamId.value) {
      actionMessage.value = `已将 ${target.username} 的角色变更为${teamRoleLabel(role)}。`
    }
  })
}

async function removeMember(target: TeamMember): Promise<void> {
  await runTeamAction(`remove:${target.user_id}`, async (id) => {
    await removeTeamMember(id, target.user_id)
    if (id !== teamId.value) return
    await loadAll()
    if (id === teamId.value) {
      actionMessage.value = `已移除成员 ${target.username}，其访问权限立即失效。`
    }
  })
}

async function leaveTeam(): Promise<void> {
  if (!auth.user) return
  const userId = auth.user.id
  await runTeamAction('leave', async (id) => {
    await removeTeamMember(id, userId)
    if (id !== teamId.value) return
    if (auth.user) await teams.loadTeams(auth.user.id, true)
    await router.push({ name: 'teams' })
  })
}

async function submitInvite(): Promise<void> {
  const username = inviteUsername.value.trim()
  const role = inviteRole.value
  const hours = inviteHours.value.trim() === '' ? undefined : Number(inviteHours.value)
  await runTeamAction('invite', async (id) => {
    await createInvitation(id, username, role, hours)
    if (id !== teamId.value) return
    const invites = await listTeamInvitations(id)
    if (id !== teamId.value) return
    invitations.value = invites
    actionMessage.value = `已向 ${username} 发出邀请。`
    inviteUsername.value = ''
  })
}

async function cancelInvite(invitation: TeamInvitation): Promise<void> {
  await runTeamAction(`cancel:${invitation.id}`, async (id) => {
    await cancelInvitation(id, invitation.id)
    if (id !== teamId.value) return
    const invites = await listTeamInvitations(id)
    if (id !== teamId.value) return
    invitations.value = invites
    actionMessage.value = `已取消对 ${invitation.invited_username} 的邀请。`
  })
}
</script>

<template>
  <div class="space-y-6">
    <div
      v-if="notFound"
      class="rounded-xl border border-warning/30 bg-warning-soft px-4 py-4 text-sm leading-6 text-warning"
      role="alert"
    >
      <p class="font-medium">团队不存在，或你已不是该团队的成员。</p>
      <p class="mt-1">被移除后访问权限立即失效；如需重新加入，请联系团队管理员再次邀请。</p>
      <RouterLink to="/teams" class="btn mt-3">返回我的团队</RouterLink>
    </div>

    <div
      v-else-if="loadError"
      class="flex flex-col gap-3 rounded-xl border border-danger/30 bg-danger-soft px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
      role="alert"
    >
      <p class="text-sm text-danger">{{ loadError }}</p>
      <button type="button" class="btn shrink-0" @click="loadAll">重试</button>
    </div>

    <template v-else>
      <section class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div class="min-w-0">
          <p class="text-xs font-medium tracking-[0.14em] text-ink-subtle uppercase">团队</p>
          <h2
            class="mt-1 flex flex-wrap items-center gap-2 text-xl font-semibold tracking-tight text-ink"
          >
            <span class="truncate">{{ team?.name ?? '加载中…' }}</span>
            <span v-if="permissions" class="badge">{{ teamRoleLabel(permissions.role) }}</span>
          </h2>
          <p v-if="team?.description" class="mt-1.5 max-w-2xl text-sm leading-6 text-ink-muted">
            {{ team.description }}
          </p>
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <button type="button" class="btn" :disabled="loading || busy !== null" @click="loadAll">
            <AppIcon name="refresh" :size="15" :class="loading ? 'animate-spin' : ''" />
            刷新
          </button>
          <button
            v-if="permissions?.can_leave"
            type="button"
            class="btn"
            :disabled="!actionsEnabled"
            @click="leaveTeam"
          >
            <AppIcon name="logout" :size="15" />
            退出团队
          </button>
        </div>
      </section>

      <div
        v-if="actionError"
        class="rounded-xl border border-danger/30 bg-danger-soft px-4 py-3 text-sm text-danger"
        role="alert"
      >
        {{ actionError }}
      </div>
      <div
        v-if="actionMessage"
        class="rounded-xl border border-accent/30 bg-accent-soft px-4 py-3 text-sm text-accent-strong"
        role="status"
      >
        {{ actionMessage }}
      </div>

      <section class="card">
        <div class="card-header">
          <div>
            <h3 class="text-sm font-semibold text-ink">成员列表</h3>
            <p class="mt-0.5 text-xs text-ink-muted">
              角色仅在本团队内生效；所有者不可被管理员变更或移除。
            </p>
          </div>
          <span class="badge">{{ members.length }} 名成员</span>
        </div>

        <p v-if="loading && members.length === 0" class="px-5 py-6 text-sm text-ink-muted">
          加载中…
        </p>

        <ul v-else class="divide-y divide-line">
          <li
            v-for="member in members"
            :key="member.user_id"
            class="flex flex-col gap-3 px-5 py-3.5 sm:flex-row sm:items-center sm:justify-between"
            :data-user-id="member.user_id"
          >
            <div class="min-w-0">
              <p class="flex flex-wrap items-center gap-2 text-sm font-medium text-ink">
                <span class="truncate">{{ member.display_name || member.username }}</span>
                <span class="font-mono text-xs text-ink-subtle">@{{ member.username }}</span>
                <span v-if="member.user_id === auth.user?.id" class="badge">我</span>
              </p>
              <p class="mt-0.5 text-xs text-ink-subtle">
                加入于 {{ formatDateTime(member.joined_at) }}
              </p>
            </div>
            <div class="flex shrink-0 items-center gap-2">
              <span class="badge">{{ teamRoleLabel(member.role) }}</span>
              <select
                v-if="roleOptions(member).length > 0"
                class="rounded-lg border border-line bg-surface px-2 py-1.5 text-xs text-ink focus:border-accent focus:outline-none"
                :value="member.role"
                :disabled="!actionsEnabled"
                :aria-label="`变更 ${member.username} 的角色`"
                @change="changeRole(member, ($event.target as HTMLSelectElement).value as TeamRole)"
              >
                <option v-for="role in roleOptions(member)" :key="role" :value="role">
                  {{ teamRoleLabel(role) }}
                </option>
              </select>
              <button
                v-if="canRemove(member)"
                type="button"
                class="btn px-2.5 py-1.5 text-xs"
                :disabled="!actionsEnabled"
                @click="removeMember(member)"
              >
                移除
              </button>
            </div>
          </li>
        </ul>
      </section>

      <section v-if="permissions?.can_manage_invitations" class="card">
        <div class="card-header">
          <div>
            <h3 class="text-sm font-semibold text-ink">邀请成员</h3>
            <p class="mt-0.5 text-xs text-ink-muted">
              仅限已存在的账号；可邀请为管理员或成员，所有者只能由现有所有者变更产生。
            </p>
          </div>
        </div>
        <form class="card-body space-y-3" @submit.prevent="submitInvite">
          <div class="grid gap-3 sm:grid-cols-3">
            <div>
              <label for="invite-username" class="field-label">用户名</label>
              <input
                id="invite-username"
                v-model="inviteUsername"
                type="text"
                required
                class="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink focus:border-accent focus:outline-none"
              />
            </div>
            <div>
              <label for="invite-role" class="field-label">团队角色</label>
              <select
                id="invite-role"
                v-model="inviteRole"
                class="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink focus:border-accent focus:outline-none"
              >
                <option value="member">成员</option>
                <option value="admin">管理员</option>
              </select>
            </div>
            <div>
              <label for="invite-hours" class="field-label">有效期（小时，1-168）</label>
              <input
                id="invite-hours"
                v-model="inviteHours"
                type="number"
                min="1"
                max="168"
                class="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink focus:border-accent focus:outline-none"
              />
            </div>
          </div>
          <button
            type="submit"
            class="btn btn-primary"
            :disabled="!actionsEnabled || !inviteUsername.trim()"
          >
            <AppIcon name="plus" :size="15" />
            发出邀请
          </button>
        </form>

        <div class="border-t border-line">
          <div class="card-header">
            <h4 class="text-sm font-semibold text-ink">待处理邀请</h4>
            <span class="badge">{{ pendingInvitations.length }}</span>
          </div>
          <p v-if="pendingInvitations.length === 0" class="px-5 py-4 text-sm text-ink-muted">
            暂无待处理邀请。
          </p>
          <ul v-else class="divide-y divide-line">
            <li
              v-for="invitation in pendingInvitations"
              :key="invitation.id"
              class="flex flex-col gap-2 px-5 py-3 sm:flex-row sm:items-center sm:justify-between"
            >
              <div class="min-w-0 text-sm text-ink">
                <span class="font-medium">{{ invitation.invited_username }}</span>
                <span class="ml-2 badge">{{ teamRoleLabel(invitation.role) }}</span>
                <p class="mt-0.5 text-xs text-ink-subtle">
                  有效期至 {{ formatDateTime(invitation.expires_at) }}
                </p>
              </div>
              <button
                type="button"
                class="btn shrink-0 px-2.5 py-1.5 text-xs"
                :disabled="!actionsEnabled"
                @click="cancelInvite(invitation)"
              >
                取消邀请
              </button>
            </li>
          </ul>
        </div>

        <div v-if="historyInvitations.length > 0" class="border-t border-line">
          <div class="card-header">
            <h4 class="text-sm font-semibold text-ink">邀请历史</h4>
          </div>
          <ul class="divide-y divide-line">
            <li
              v-for="invitation in historyInvitations"
              :key="invitation.id"
              class="flex items-center justify-between gap-3 px-5 py-2.5 text-sm"
            >
              <span class="truncate text-ink">{{ invitation.invited_username }}</span>
              <span class="badge shrink-0">{{ invitationStatusLabel(invitation.status) }}</span>
            </li>
          </ul>
        </div>
      </section>
    </template>
  </div>
</template>
