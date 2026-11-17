<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'

import { ApiError } from '@/api/client'
import AppIcon from '@/components/ui/AppIcon.vue'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
import { formatDateTime, teamRoleLabel } from '@/utils/format'

const auth = useAuthStore()
const teams = useTeamsStore()
const router = useRouter()

const name = ref('')
const description = ref('')
const creating = ref(false)
const createError = ref<string | null>(null)
const createdName = ref<string | null>(null)

onMounted(() => {
  if (auth.user) void teams.loadTeams(auth.user.id, true)
})

async function submitCreate(): Promise<void> {
  if (creating.value) return
  creating.value = true
  createError.value = null
  createdName.value = null
  try {
    const team = await teams.createTeam(name.value.trim(), description.value.trim() || undefined)
    createdName.value = team.name
    name.value = ''
    description.value = ''
  } catch (reason) {
    createError.value = reason instanceof ApiError ? reason.message : '无法连接到 Nexora API'
  } finally {
    creating.value = false
  }
}

function enterTeam(teamId: string): void {
  teams.selectTeam(teamId)
  void router.push({ name: 'team-members', params: { teamId } })
}
</script>

<template>
  <div class="space-y-6">
    <section class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <p class="text-xs font-medium tracking-[0.14em] text-ink-subtle uppercase">Phase 02</p>
        <h2 class="mt-1 text-xl font-semibold tracking-tight text-ink">我的团队</h2>
        <p class="mt-1.5 max-w-2xl text-sm leading-6 text-ink-muted">
          你可以加入多个团队，在每个团队中拥有独立角色；团队之间的数据相互隔离。
        </p>
      </div>
      <button
        type="button"
        class="btn shrink-0"
        :disabled="teams.loading"
        @click="auth.user && teams.loadTeams(auth.user.id, true)"
      >
        <AppIcon name="refresh" :size="15" :class="teams.loading ? 'animate-spin' : ''" />
        刷新
      </button>
    </section>

    <div
      v-if="teams.error"
      class="flex flex-col gap-3 rounded-xl border border-danger/30 bg-danger-soft px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
      role="alert"
    >
      <p class="text-sm text-danger">{{ teams.error }}</p>
      <button
        type="button"
        class="btn shrink-0"
        @click="auth.user && teams.loadTeams(auth.user.id, true)"
      >
        重试
      </button>
    </div>

    <section class="card">
      <div class="card-header">
        <div>
          <h3 class="text-sm font-semibold text-ink">创建团队</h3>
          <p class="mt-0.5 text-xs text-ink-muted">创建者自动成为该团队的所有者（owner）。</p>
        </div>
      </div>
      <form class="card-body space-y-3" @submit.prevent="submitCreate">
        <div
          v-if="createError"
          class="rounded-lg border border-danger/30 bg-danger-soft px-3 py-2 text-sm text-danger"
          role="alert"
        >
          {{ createError }}
        </div>
        <div
          v-if="createdName"
          class="rounded-lg border border-accent/30 bg-accent-soft px-3 py-2 text-sm text-accent-strong"
          role="status"
        >
          团队「{{ createdName }}」已创建，你已成为所有者。
        </div>
        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label for="team-name" class="field-label">团队名称（1-64 字符）</label>
            <input
              id="team-name"
              v-model="name"
              type="text"
              required
              maxlength="64"
              class="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink focus:border-accent focus:outline-none"
            />
          </div>
          <div>
            <label for="team-description" class="field-label">描述（可选，≤500 字符）</label>
            <input
              id="team-description"
              v-model="description"
              type="text"
              maxlength="500"
              class="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink focus:border-accent focus:outline-none"
            />
          </div>
        </div>
        <button type="submit" class="btn btn-primary" :disabled="creating || !name.trim()">
          <AppIcon name="plus" :size="15" />
          {{ creating ? '创建中' : '创建团队' }}
        </button>
      </form>
    </section>

    <section class="card">
      <div class="card-header">
        <h3 class="text-sm font-semibold text-ink">团队列表</h3>
        <span class="badge">{{ teams.teams.length }} 个团队</span>
      </div>

      <p v-if="teams.loading" class="px-5 py-6 text-sm text-ink-muted">加载中…</p>

      <div
        v-else-if="teams.teams.length === 0 && !teams.error"
        class="px-5 py-6 text-sm leading-6 text-ink-muted"
      >
        你还没有加入任何团队。创建一个新团队，或前往
        <RouterLink to="/invitations" class="text-accent underline">邀请收件箱</RouterLink>
        接受管理员的邀请。
      </div>

      <ul v-else class="divide-y divide-line">
        <li
          v-for="team in teams.teams"
          :key="team.id"
          class="flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-center sm:justify-between"
          :data-team-id="team.id"
        >
          <div class="min-w-0">
            <p class="flex flex-wrap items-center gap-2 text-sm font-medium text-ink">
              <span class="truncate">{{ team.name }}</span>
              <span class="badge">{{ teamRoleLabel(team.my_role) }}</span>
              <span v-if="team.id === teams.currentTeamId" class="badge">当前团队</span>
            </p>
            <p v-if="team.description" class="mt-1 truncate text-xs text-ink-muted">
              {{ team.description }}
            </p>
            <p class="mt-1 text-xs text-ink-subtle">创建于 {{ formatDateTime(team.created_at) }}</p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <button
              v-if="team.id !== teams.currentTeamId"
              type="button"
              class="btn"
              @click="teams.selectTeam(team.id)"
            >
              设为当前
            </button>
            <button type="button" class="btn btn-primary" @click="enterTeam(team.id)">
              <AppIcon name="users" :size="15" />
              成员与邀请
            </button>
          </div>
        </li>
      </ul>
    </section>
  </div>
</template>
