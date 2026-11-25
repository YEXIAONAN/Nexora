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
import AlertBanner from '@/components/ui/AlertBanner.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
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
  <div class="page-stack">
    <PageHeader
      eyebrow="协作"
      title="邀请收件箱"
      description="这里列出发送给你的有效团队邀请；过期邀请会自动消失，接受后立即成为该团队成员。"
    >
      <template #actions>
        <button type="button" class="btn" :disabled="loading" @click="load">
          <AppIcon name="refresh" :size="15" :class="loading ? 'animate-spin' : ''" />
          刷新
        </button>
      </template>
    </PageHeader>

    <AlertBanner v-if="loadError" tone="danger" :message="loadError">
      <template #action>
        <button type="button" class="btn btn-sm" @click="load">重试</button>
      </template>
    </AlertBanner>

    <AlertBanner v-if="actionError" tone="danger" :message="actionError" />
    <AlertBanner v-if="actionMessage" tone="success" :message="actionMessage">
      <template #action>
        <RouterLink to="/teams" class="btn btn-sm">查看我的团队</RouterLink>
      </template>
    </AlertBanner>

    <section class="card">
      <div class="card-header">
        <div>
          <h3 class="card-title">有效邀请</h3>
          <p class="card-description">接受邀请后，团队会立即出现在你的团队列表中。</p>
        </div>
        <span class="badge">{{ invitations.length }}</span>
      </div>

      <div v-if="loading" class="space-y-2 p-4">
        <div v-for="i in 2" :key="i" class="skeleton h-16" />
      </div>

      <EmptyState
        v-else-if="invitations.length === 0 && !loadError"
        icon="inbox"
        title="暂无有效邀请"
        description="团队管理员可以通过用户名邀请你加入。"
      />

      <ul v-else class="divide-y divide-line">
        <li
          v-for="invitation in invitations"
          :key="invitation.id"
          class="list-row"
          :data-invitation-id="invitation.id"
        >
          <div class="min-w-0">
            <p class="flex flex-wrap items-center gap-2 text-body font-medium text-ink">
              <span class="truncate">{{ invitation.team_name ?? '团队' }}</span>
              <span class="badge">{{ teamRoleLabel(invitation.role) }}</span>
            </p>
            <p class="mt-1 text-caption text-ink-subtle">
              有效期至 {{ formatDateTime(invitation.expires_at) }}
            </p>
          </div>
          <div class="flex shrink-0 flex-wrap items-center gap-2">
            <button
              type="button"
              class="btn btn-primary btn-sm"
              :disabled="busy !== null"
              @click="respond(invitation, true)"
            >
              接受
            </button>
            <button
              type="button"
              class="btn btn-sm"
              :disabled="busy !== null"
              @click="respond(invitation, false)"
            >
              拒绝
            </button>
            <button
              v-if="teams.teams.some((t) => t.id === invitation.team_id)"
              type="button"
              class="btn btn-ghost btn-sm"
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
