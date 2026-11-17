<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'

import { ApiError } from '@/api/client'
import {
  acceptInvitation,
  declineInvitation,
  listMyInvitations,
  type TeamInvitation,
} from '@/api/teams'
import AppIcon from '@/components/ui/AppIcon.vue'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
import { formatDateTime, teamRoleLabel } from '@/utils/format'

const auth = useAuthStore()
const teams = useTeamsStore()
const router = useRouter()

const invitations = ref<TeamInvitation[]>([])
const loading = ref(false)
const loadError = ref<string | null>(null)
const actionError = ref<string | null>(null)
const actionMessage = ref<string | null>(null)
const busy = ref<string | null>(null)

function messageOf(reason: unknown): string {
  return reason instanceof ApiError ? reason.message : '无法连接到 Nexora API'
}

async function load(): Promise<void> {
  loading.value = true
  loadError.value = null
  try {
    invitations.value = await listMyInvitations()
  } catch (reason) {
    loadError.value = messageOf(reason)
  } finally {
    loading.value = false
  }
}

onMounted(load)

async function respond(invitation: TeamInvitation, accept: boolean): Promise<void> {
  if (busy.value) return
  busy.value = invitation.id
  actionError.value = null
  actionMessage.value = null
  try {
    if (accept) {
      await acceptInvitation(invitation.id)
      if (auth.user) await teams.loadTeams(auth.user.id, true)
      actionMessage.value = `已加入团队「${invitation.team_name ?? ''}」。`
    } else {
      await declineInvitation(invitation.id)
      actionMessage.value = `已拒绝团队「${invitation.team_name ?? ''}」的邀请。`
    }
    invitations.value = await listMyInvitations()
  } catch (reason) {
    actionError.value = messageOf(reason)
  } finally {
    busy.value = null
  }
}

function openTeam(invitation: TeamInvitation): void {
  teams.selectTeam(invitation.team_id)
  void router.push({ name: 'team-members', params: { teamId: invitation.team_id } })
}
</script>

<template>
  <div class="space-y-6">
    <section class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <p class="text-xs font-medium tracking-[0.14em] text-ink-subtle uppercase">Phase 02</p>
        <h2 class="mt-1 text-xl font-semibold tracking-tight text-ink">邀请收件箱</h2>
        <p class="mt-1.5 max-w-2xl text-sm leading-6 text-ink-muted">
          这里列出发送给你的有效团队邀请；过期邀请会自动消失，接受后立即成为该团队成员。
        </p>
      </div>
      <button type="button" class="btn shrink-0" :disabled="loading" @click="load">
        <AppIcon name="refresh" :size="15" :class="loading ? 'animate-spin' : ''" />
        刷新
      </button>
    </section>

    <div
      v-if="loadError"
      class="flex flex-col gap-3 rounded-xl border border-danger/30 bg-danger-soft px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
      role="alert"
    >
      <p class="text-sm text-danger">{{ loadError }}</p>
      <button type="button" class="btn shrink-0" @click="load">重试</button>
    </div>

    <div
      v-if="actionError"
      class="rounded-xl border border-danger/30 bg-danger-soft px-4 py-3 text-sm text-danger"
      role="alert"
    >
      {{ actionError }}
    </div>
    <div
      v-if="actionMessage"
      class="flex flex-col gap-3 rounded-xl border border-accent/30 bg-accent-soft px-4 py-3 text-sm text-accent-strong sm:flex-row sm:items-center sm:justify-between"
      role="status"
    >
      <span>{{ actionMessage }}</span>
      <RouterLink to="/teams" class="btn shrink-0">查看我的团队</RouterLink>
    </div>

    <section class="card">
      <div class="card-header">
        <h3 class="text-sm font-semibold text-ink">有效邀请</h3>
        <span class="badge">{{ invitations.length }}</span>
      </div>

      <p v-if="loading" class="px-5 py-6 text-sm text-ink-muted">加载中…</p>

      <p
        v-else-if="invitations.length === 0 && !loadError"
        class="px-5 py-6 text-sm text-ink-muted"
      >
        暂无有效邀请。团队管理员可以通过用户名邀请你加入。
      </p>

      <ul v-else class="divide-y divide-line">
        <li
          v-for="invitation in invitations"
          :key="invitation.id"
          class="flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-center sm:justify-between"
          :data-invitation-id="invitation.id"
        >
          <div class="min-w-0">
            <p class="flex flex-wrap items-center gap-2 text-sm font-medium text-ink">
              <span class="truncate">{{ invitation.team_name ?? '团队' }}</span>
              <span class="badge">{{ teamRoleLabel(invitation.role) }}</span>
            </p>
            <p class="mt-1 text-xs text-ink-subtle">
              有效期至 {{ formatDateTime(invitation.expires_at) }}
            </p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <button
              type="button"
              class="btn btn-primary"
              :disabled="busy !== null"
              @click="respond(invitation, true)"
            >
              接受
            </button>
            <button
              type="button"
              class="btn"
              :disabled="busy !== null"
              @click="respond(invitation, false)"
            >
              拒绝
            </button>
            <button
              v-if="teams.teams.some((t) => t.id === invitation.team_id)"
              type="button"
              class="btn btn-ghost"
              :disabled="busy !== null"
              @click="openTeam(invitation)"
            >
              查看团队
            </button>
          </div>
        </li>
      </ul>
    </section>
  </div>
</template>
