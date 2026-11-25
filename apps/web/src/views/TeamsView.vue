<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'

import { listUsers, type User } from '@/api/auth'
import { ApiError } from '@/api/client'
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

const isAdmin = computed(() => auth.isPlatformAdmin)
const name = ref('')
const description = ref('')
const ownerId = ref('')
const owners = ref<User[]>([])
const ownersError = ref<string | null>(null)
const creating = ref(false)
const createError = ref<string | null>(null)
const createdName = ref<string | null>(null)

onMounted(async () => {
  if (auth.user) void teams.loadTeams(auth.user.id, true)
  if (isAdmin.value) await loadOwners()
})

async function loadOwners(): Promise<void> {
  ownersError.value = null
  try {
    const page = await listUsers('', 100)
    owners.value = page.items.filter((user) => user.is_active !== false)
    if (!ownerId.value) ownerId.value = auth.user?.id ?? owners.value[0]?.id ?? ''
  } catch (reason) {
    ownersError.value = reason instanceof ApiError ? reason.message : '无法加载用户列表'
  }
}

function ownerName(id: string): string {
  const user = owners.value.find((item) => item.id === id)
  return user?.display_name || user?.username || '未选择'
}

async function submitCreate(): Promise<void> {
  if (creating.value || !ownerId.value) return
  creating.value = true
  createError.value = null
  createdName.value = null
  try {
    const team = await teams.createTeam(
      name.value.trim(),
      description.value.trim() || undefined,
      ownerId.value,
    )
    createdName.value = `${team.name}（负责人：${ownerName(ownerId.value)}）`
    name.value = ''
    description.value = ''
  } catch (reason) {
    if (reason instanceof ApiError && reason.fields) {
      createError.value = Object.values(reason.fields).join('；')
    } else {
      createError.value = reason instanceof ApiError ? reason.message : '无法连接到 Nexora API'
    }
  } finally {
    creating.value = false
  }
}

function enterProjects(teamId: string): void {
  teams.selectTeam(teamId)
  void router.push({ name: 'projects', params: { teamId } })
}

function enterTeam(teamId: string): void {
  teams.selectTeam(teamId)
  void router.push({ name: 'team-members', params: { teamId } })
}
</script>

<template>
  <div class="page-stack">
    <PageHeader
      eyebrow="团队"
      title="我的团队"
      description="你可以加入多个团队，在每个团队中拥有独立角色；团队之间的数据相互隔离。"
    >
      <template #actions>
        <button
          type="button"
          class="btn"
          :disabled="teams.loading"
          @click="auth.user && teams.loadTeams(auth.user.id, true)"
        >
          <AppIcon name="refresh" :size="15" :class="teams.loading ? 'animate-spin' : ''" />
          刷新
        </button>
      </template>
    </PageHeader>

    <AlertBanner v-if="teams.error" tone="danger" :message="teams.error">
      <template #action>
        <button
          type="button"
          class="btn btn-sm"
          @click="auth.user && teams.loadTeams(auth.user.id, true)"
        >
          重试
        </button>
      </template>
    </AlertBanner>

    <section v-if="isAdmin" class="card">
      <div class="card-header">
        <div>
          <h3 class="card-title">创建团队</h3>
          <p class="card-description">
            只有平台管理员可以创建团队，并指定一名团队负责人（owner）。
          </p>
        </div>
      </div>
      <form class="card-body space-y-3" @submit.prevent="submitCreate">
        <AlertBanner v-if="createError" tone="danger" :message="createError" />
        <AlertBanner
          v-if="createdName"
          tone="success"
          :message="`团队「${createdName}」已创建。`"
        />
        <AlertBanner v-if="ownersError" tone="danger" :message="ownersError">
          <template #action>
            <button type="button" class="btn btn-sm" @click="loadOwners">重试</button>
          </template>
        </AlertBanner>
        <div class="grid gap-3 sm:grid-cols-3">
          <div>
            <label for="team-name" class="field-label">团队名称（1-64 字符）</label>
            <input
              id="team-name"
              v-model="name"
              type="text"
              required
              maxlength="64"
              class="form-input mt-1"
            />
          </div>
          <div>
            <label for="team-description" class="field-label">描述（可选，≤500 字符）</label>
            <input
              id="team-description"
              v-model="description"
              type="text"
              maxlength="500"
              class="form-input mt-1"
            />
          </div>
          <div>
            <label for="team-owner" class="field-label">团队负责人</label>
            <select id="team-owner" v-model="ownerId" required class="form-input mt-1">
              <option value="" disabled>请选择负责人</option>
              <option v-for="user in owners" :key="user.id" :value="user.id">
                {{ user.display_name || user.username }} @{{ user.username }}
              </option>
            </select>
          </div>
        </div>
        <button
          type="submit"
          class="btn btn-primary"
          :disabled="creating || !name.trim() || !ownerId || owners.length === 0"
        >
          <AppIcon name="plus" :size="15" />
          {{ creating ? '创建中' : '创建团队' }}
        </button>
      </form>
    </section>

    <section v-else class="card">
      <div class="card-header">
        <div>
          <h3 class="card-title">团队由平台管理员创建</h3>
          <p class="card-description">
            普通账号不能自助创建团队。请联系平台管理员创建团队并把你设为负责人，或邀请你加入已有团队。
          </p>
        </div>
      </div>
      <div class="card-body flex flex-wrap items-center justify-between gap-3">
        <p class="text-sm text-ink-muted">已有邀请？打开邀请收件箱接受团队邀请后即可进入团队。</p>
        <RouterLink to="/invitations" class="btn btn-sm">查看邀请</RouterLink>
      </div>
    </section>

    <section class="card">
      <div class="card-header">
        <div>
          <h3 class="card-title">团队列表</h3>
          <p class="card-description">当前团队决定侧边栏与工作台展示的数据范围。</p>
        </div>
        <span class="badge">{{ teams.teams.length }} 个团队</span>
      </div>

      <div v-if="teams.loading" class="space-y-2 p-4">
        <div v-for="i in 3" :key="i" class="skeleton h-16" />
      </div>

      <EmptyState
        v-else-if="teams.teams.length === 0 && !teams.error"
        icon="users"
        title="你还没有加入任何团队。"
        description="创建一个新团队，或前往邀请收件箱接受管理员的邀请。"
      >
        <template #action>
          <RouterLink to="/invitations" class="btn btn-sm">邀请收件箱</RouterLink>
        </template>
      </EmptyState>

      <ul v-else class="divide-y divide-line">
        <li v-for="team in teams.teams" :key="team.id" class="list-row" :data-team-id="team.id">
          <div class="min-w-0">
            <p class="flex flex-wrap items-center gap-2 text-body font-medium text-ink">
              <span class="truncate">{{ team.name }}</span>
              <span class="badge">{{ teamRoleLabel(team.my_role) }}</span>
              <span v-if="team.id === teams.currentTeamId" class="badge badge-accent"
                >当前团队</span
              >
            </p>
            <p v-if="team.description" class="mt-1 truncate text-caption text-ink-muted">
              {{ team.description }}
            </p>
            <p class="mt-1 text-caption text-ink-subtle">
              创建于 {{ formatDateTime(team.created_at) }}
            </p>
          </div>
          <div class="flex shrink-0 flex-wrap items-center gap-2">
            <button
              v-if="team.id !== teams.currentTeamId"
              type="button"
              class="btn btn-sm"
              @click="teams.selectTeam(team.id)"
            >
              设为当前
            </button>
            <button type="button" class="btn btn-sm" @click="enterTeam(team.id)">
              <AppIcon name="users" :size="14" />
              成员与邀请
            </button>
            <button type="button" class="btn btn-primary btn-sm" @click="enterProjects(team.id)">
              项目
            </button>
          </div>
        </li>
      </ul>
    </section>
  </div>
</template>
