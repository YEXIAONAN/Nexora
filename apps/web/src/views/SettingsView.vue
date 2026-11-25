<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import { ApiError } from '@/api/client'
import { updateTeam } from '@/api/teams'
import EmptyState from '@/components/ui/EmptyState.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
import { useUiStore } from '@/stores/ui'

type Tab = 'profile' | 'security' | 'team'

const auth = useAuthStore()
const teams = useTeamsStore()
const ui = useUiStore()
const router = useRouter()

const tab = ref<Tab>('profile')
const profile = reactive({ displayName: '', email: '' })
const profileSaving = ref(false)
const profileErrors = ref<Record<string, string>>({})

const password = reactive({ current: '', next: '', confirm: '' })
const passwordSaving = ref(false)
const passwordErrors = ref<Record<string, string>>({})

const teamForm = reactive({ name: '', description: '' })
const teamSaving = ref(false)
const teamErrors = ref<Record<string, string>>({})

const currentTeam = computed(() => teams.currentTeam)
const canManageTeam = computed(
  () => currentTeam.value?.my_role === 'owner' || currentTeam.value?.my_role === 'admin',
)

function fillProfile(): void {
  profile.displayName = auth.user?.display_name ?? ''
  profile.email = auth.user?.email ?? ''
}

function fillTeam(): void {
  teamForm.name = currentTeam.value?.name ?? ''
  teamForm.description = currentTeam.value?.description ?? ''
}

watch(
  () => auth.user,
  () => fillProfile(),
  { immediate: true },
)
watch(currentTeam, fillTeam, { immediate: true })

onMounted(async () => {
  if (auth.user) await teams.loadTeams(auth.user.id)
  fillTeam()
})

async function saveProfile(): Promise<void> {
  if (profileSaving.value) return
  profileSaving.value = true
  profileErrors.value = {}
  try {
    await auth.updateProfile(profile.displayName, profile.email)
    ui.notify('个人资料已更新')
  } catch (reason) {
    if (reason instanceof ApiError) {
      profileErrors.value = reason.fields ?? {}
      ui.notify(reason.message, 'error')
    } else {
      ui.notify('保存失败，请稍后重试', 'error')
    }
  } finally {
    profileSaving.value = false
  }
}

async function savePassword(): Promise<void> {
  if (passwordSaving.value) return
  passwordErrors.value = {}
  if (new TextEncoder().encode(password.next).length < 10) {
    passwordErrors.value = { new_password: '密码至少 10 个字符' }
    return
  }
  if (password.next !== password.confirm) {
    passwordErrors.value = { confirm_password: '两次输入的新密码不一致' }
    return
  }
  passwordSaving.value = true
  try {
    await auth.changePassword(password.current, password.next)
    ui.notify('密码已更新，请使用新密码重新登录')
    await router.push({ name: 'login', query: { expired: '1' } })
  } catch (reason) {
    if (reason instanceof ApiError) {
      passwordErrors.value = reason.fields ?? {}
      ui.notify(reason.message, 'error')
    } else {
      ui.notify('修改密码失败', 'error')
    }
  } finally {
    passwordSaving.value = false
  }
}

async function saveTeam(): Promise<void> {
  if (!currentTeam.value || teamSaving.value) return
  teamSaving.value = true
  teamErrors.value = {}
  try {
    const updated = await updateTeam(currentTeam.value.id, teamForm.name, teamForm.description)
    teams.applyTeamUpdate(updated)
    ui.notify('团队设置已更新')
  } catch (reason) {
    if (reason instanceof ApiError) {
      teamErrors.value = reason.fields ?? {}
      ui.notify(reason.message, 'error')
    } else {
      ui.notify('保存团队设置失败', 'error')
    }
  } finally {
    teamSaving.value = false
  }
}
</script>

<template>
  <div class="page-stack">
    <PageHeader
      eyebrow="设置"
      title="账号与团队"
      description="管理个人资料、登录安全，以及你拥有权限的团队信息。"
    />

    <div class="tabs w-fit" role="tablist" aria-label="设置分类">
      <button
        type="button"
        role="tab"
        class="tab"
        :class="tab === 'profile' ? 'tab-active' : ''"
        @click="tab = 'profile'"
      >
        个人资料
      </button>
      <button
        type="button"
        role="tab"
        class="tab"
        :class="tab === 'security' ? 'tab-active' : ''"
        @click="tab = 'security'"
      >
        账号安全
      </button>
      <button
        v-if="currentTeam"
        type="button"
        role="tab"
        class="tab"
        :class="tab === 'team' ? 'tab-active' : ''"
        @click="tab = 'team'"
      >
        团队设置
      </button>
    </div>

    <section
      v-if="tab === 'profile'"
      class="grid grid-cols-1 gap-5 lg:grid-cols-[minmax(0,1fr)_260px]"
    >
      <form class="card" @submit.prevent="saveProfile">
        <div class="card-header">
          <div>
            <h3 class="section-title">个人资料</h3>
            <p class="mt-0.5 text-xs text-ink-muted">显示名称会出现在团队成员与审核记录中。</p>
          </div>
        </div>
        <div class="card-body space-y-4">
          <div>
            <label for="settings-display-name" class="field-label">显示名称</label>
            <input
              id="settings-display-name"
              v-model="profile.displayName"
              type="text"
              maxlength="64"
              class="form-input mt-1"
            />
            <p v-if="profileErrors.display_name" class="field-error">
              {{ profileErrors.display_name }}
            </p>
          </div>
          <div>
            <label for="settings-email" class="field-label">邮箱（可选）</label>
            <input
              id="settings-email"
              v-model="profile.email"
              type="email"
              maxlength="254"
              class="form-input mt-1"
              placeholder="you@example.com"
            />
            <p v-if="profileErrors.email" class="field-error">{{ profileErrors.email }}</p>
          </div>
          <div class="flex justify-end">
            <button type="submit" class="btn btn-primary btn-sm" :disabled="profileSaving">
              {{ profileSaving ? '保存中' : '保存资料' }}
            </button>
          </div>
        </div>
      </form>

      <aside class="card h-fit">
        <div class="card-header">
          <h3 class="section-title">账号信息</h3>
        </div>
        <dl class="divide-y divide-line">
          <div class="px-5 py-3">
            <dt class="text-meta text-ink-subtle">用户名</dt>
            <dd class="mt-0.5 font-mono text-sm text-ink">{{ auth.user?.username }}</dd>
          </div>
          <div class="px-5 py-3">
            <dt class="text-meta text-ink-subtle">平台角色</dt>
            <dd class="mt-0.5 text-sm text-ink">
              {{
                auth.user?.system_root
                  ? 'ROOT'
                  : auth.user?.platform_admin
                    ? '平台管理员'
                    : '普通用户'
              }}
            </dd>
          </div>
          <div class="px-5 py-3">
            <dt class="text-meta text-ink-subtle">账号状态</dt>
            <dd class="mt-0.5 flex items-center gap-1.5 text-sm text-ink">
              <span
                class="h-1.5 w-1.5 rounded-full"
                :class="auth.user?.is_active === false ? 'bg-danger' : 'bg-accent'"
              />
              {{ auth.user?.is_active === false ? '已停用' : '正常' }}
            </dd>
          </div>
          <div class="px-5 py-3">
            <dt class="text-meta text-ink-subtle">注册时间</dt>
            <dd class="mt-0.5 text-sm text-ink">
              {{ auth.user?.created_at ? auth.user.created_at.slice(0, 10) : '—' }}
            </dd>
          </div>
        </dl>
      </aside>
    </section>

    <section v-else-if="tab === 'security'" class="card max-w-xl">
      <div class="card-header">
        <div>
          <h3 class="section-title">修改密码</h3>
          <p class="mt-0.5 text-xs text-ink-muted">修改成功后所有会话将失效，需要重新登录。</p>
        </div>
      </div>
      <form class="card-body space-y-4" @submit.prevent="savePassword">
        <div>
          <label for="current-password" class="field-label">当前密码</label>
          <input
            id="current-password"
            v-model="password.current"
            type="password"
            autocomplete="current-password"
            required
            class="form-input mt-1"
          />
          <p v-if="passwordErrors.current_password" class="field-error">
            {{ passwordErrors.current_password }}
          </p>
        </div>
        <div>
          <label for="new-password" class="field-label">新密码</label>
          <input
            id="new-password"
            v-model="password.next"
            type="password"
            autocomplete="new-password"
            required
            class="form-input mt-1"
          />
          <p class="mt-1 text-meta text-ink-subtle">至少 10 个字符，且不能与用户名相同。</p>
          <p v-if="passwordErrors.new_password" class="field-error">
            {{ passwordErrors.new_password }}
          </p>
        </div>
        <div>
          <label for="confirm-password" class="field-label">确认新密码</label>
          <input
            id="confirm-password"
            v-model="password.confirm"
            type="password"
            autocomplete="new-password"
            required
            class="form-input mt-1"
          />
          <p v-if="passwordErrors.confirm_password" class="field-error">
            {{ passwordErrors.confirm_password }}
          </p>
        </div>
        <div class="flex justify-end">
          <button type="submit" class="btn btn-primary btn-sm" :disabled="passwordSaving">
            {{ passwordSaving ? '修改中' : '修改密码' }}
          </button>
        </div>
      </form>
    </section>

    <section v-else-if="tab === 'team'" class="card max-w-xl">
      <div class="card-header">
        <div>
          <h3 class="section-title">团队设置</h3>
          <p class="mt-0.5 text-xs text-ink-muted">{{ currentTeam?.name }}</p>
        </div>
        <span class="badge">
          {{
            currentTeam?.my_role === 'owner'
              ? 'Owner'
              : currentTeam?.my_role === 'admin'
                ? 'Admin'
                : 'Member'
          }}
        </span>
      </div>
      <form v-if="canManageTeam" class="card-body space-y-4" @submit.prevent="saveTeam">
        <div>
          <label for="team-name" class="field-label">团队名称</label>
          <input
            id="team-name"
            v-model="teamForm.name"
            type="text"
            maxlength="64"
            required
            class="form-input mt-1"
          />
          <p v-if="teamErrors.name" class="field-error">{{ teamErrors.name }}</p>
        </div>
        <div>
          <label for="team-description" class="field-label">团队描述</label>
          <textarea
            id="team-description"
            v-model="teamForm.description"
            rows="4"
            maxlength="500"
            class="form-input mt-1"
          />
          <p v-if="teamErrors.description" class="field-error">{{ teamErrors.description }}</p>
        </div>
        <div class="flex justify-end">
          <button type="submit" class="btn btn-primary btn-sm" :disabled="teamSaving">
            {{ teamSaving ? '保存中' : '保存团队设置' }}
          </button>
        </div>
      </form>
      <div v-else class="card">
        <EmptyState
          icon="shield"
          title="只有 Owner 或 Admin 可以编辑团队设置"
          description="你可以在「团队成员」页面查看成员和你的角色。"
        />
      </div>
    </section>
  </div>
</template>
