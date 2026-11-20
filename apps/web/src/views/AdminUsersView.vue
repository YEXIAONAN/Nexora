<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'

import {
  createUser,
  grantPlatformAdmin,
  listUsers,
  revokePlatformAdmin,
  revokeUserSessions,
  updateUser,
  type User,
} from '@/api/auth'
import { ApiError } from '@/api/client'
import AppIcon from '@/components/ui/AppIcon.vue'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'

const auth = useAuthStore()
const ui = useUiStore()

const users = ref<User[]>([])
const total = ref(0)
const limit = 25
const offset = ref(0)
const search = ref('')
const loading = ref(true)
const error = ref<string | null>(null)
let timer: number | null = null

const showCreate = ref(false)
const createForm = reactive({ username: '', display_name: '', email: '', password: '' })
const createErrors = ref<Record<string, string>>({})
const creating = ref(false)

const editing = ref<User | null>(null)
const editForm = reactive({ display_name: '', email: '' })
const editErrors = ref<Record<string, string>>({})
const savingEdit = ref(false)

const isRoot = computed(() => Boolean(auth.user?.system_root))
const page = computed(() => Math.floor(offset.value / limit) + 1)
const pages = computed(() => Math.max(1, Math.ceil(total.value / limit)))

async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const result = await listUsers(search.value, limit, offset.value)
    users.value = result.items
    total.value = result.total
  } catch (reason) {
    error.value = reason instanceof ApiError ? reason.message : '无法加载用户列表'
  } finally {
    loading.value = false
  }
}

function scheduleSearch(): void {
  if (timer) window.clearTimeout(timer)
  timer = window.setTimeout(() => {
    offset.value = 0
    void load()
  }, 280)
}

function openCreate(): void {
  showCreate.value = true
  createErrors.value = {}
  Object.assign(createForm, { username: '', display_name: '', email: '', password: '' })
}

async function submitCreate(): Promise<void> {
  if (creating.value) return
  creating.value = true
  createErrors.value = {}
  try {
    await createUser({
      username: createForm.username.trim().toLowerCase(),
      display_name: createForm.display_name.trim(),
      email: createForm.email.trim(),
      password: createForm.password,
    })
    ui.notify('账号已创建')
    showCreate.value = false
    offset.value = 0
    await load()
  } catch (reason) {
    if (reason instanceof ApiError) {
      createErrors.value = reason.fields ?? {}
      ui.notify(reason.message, 'error')
    } else {
      ui.notify('创建账号失败', 'error')
    }
  } finally {
    creating.value = false
  }
}

function openEdit(user: User): void {
  editing.value = user
  editForm.display_name = user.display_name
  editForm.email = user.email ?? ''
  editErrors.value = {}
}

async function submitEdit(): Promise<void> {
  if (!editing.value || savingEdit.value) return
  savingEdit.value = true
  editErrors.value = {}
  try {
    const updated = await updateUser(editing.value.id, {
      display_name: editForm.display_name,
      email: editForm.email,
    })
    users.value = users.value.map((item) => (item.id === updated.id ? updated : item))
    if (auth.user?.id === updated.id) auth.setUser(updated)
    ui.notify('用户资料已更新')
    editing.value = null
  } catch (reason) {
    if (reason instanceof ApiError) {
      editErrors.value = reason.fields ?? {}
      ui.notify(reason.message, 'error')
    } else {
      ui.notify('更新失败', 'error')
    }
  } finally {
    savingEdit.value = false
  }
}

async function toggleActive(user: User): Promise<void> {
  const next = user.is_active === false
  const ok = await ui.confirm({
    title: next ? '启用账号' : '停用账号',
    message: next
      ? `启用后 ${user.username} 可以重新登录。`
      : `停用后 ${user.username} 将立即失去访问权限，现有会话会被撤销。`,
    confirmLabel: next ? '启用' : '停用',
    tone: next ? 'default' : 'danger',
  })
  if (!ok) return
  try {
    const updated = await updateUser(user.id, { is_active: next })
    users.value = users.value.map((item) => (item.id === updated.id ? updated : item))
    ui.notify(next ? '账号已启用' : '账号已停用')
  } catch (reason) {
    ui.notify(reason instanceof ApiError ? reason.message : '操作失败', 'error')
  }
}

async function toggleAdmin(user: User): Promise<void> {
  const next = !user.platform_admin
  const ok = await ui.confirm({
    title: next ? '授予平台管理员' : '撤销平台管理员',
    message: next
      ? `${user.username} 将可以管理所有账号和查看系统状态。`
      : `${user.username} 将失去平台管理员权限。`,
    confirmLabel: next ? '授予' : '撤销',
    tone: next ? 'default' : 'danger',
  })
  if (!ok) return
  try {
    if (next) await grantPlatformAdmin(user.id)
    else await revokePlatformAdmin(user.id)
    await load()
    ui.notify(next ? '已授予平台管理员' : '已撤销平台管理员')
  } catch (reason) {
    ui.notify(reason instanceof ApiError ? reason.message : '操作失败', 'error')
  }
}

async function revokeSessions(user: User): Promise<void> {
  const ok = await ui.confirm({
    title: '撤销所有会话',
    message: `确认让 ${user.username} 在所有设备上重新登录？`,
    confirmLabel: '撤销会话',
    tone: 'danger',
  })
  if (!ok) return
  try {
    const count = await revokeUserSessions(user.id)
    ui.notify(`已撤销 ${count} 个会话`)
  } catch (reason) {
    ui.notify(reason instanceof ApiError ? reason.message : '操作失败', 'error')
  }
}

function changePage(delta: number): void {
  offset.value = Math.max(0, Math.min((pages.value - 1) * limit, offset.value + delta * limit))
  void load()
}

onMounted(load)
</script>

<template>
  <div class="space-y-5">
    <div class="page-header">
      <div>
        <p class="eyebrow">系统管理</p>
        <h2 class="page-title mt-1">用户管理</h2>
        <p class="page-subtitle">创建、搜索和管理账号状态。普通用户不会自动获得平台管理员权限。</p>
      </div>
      <button type="button" class="btn btn-primary" @click="openCreate">
        <AppIcon name="plus" :size="15" />
        创建用户
      </button>
    </div>

    <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div class="relative max-w-sm">
        <AppIcon
          name="search"
          :size="15"
          class="pointer-events-none absolute top-2.5 left-3 text-ink-subtle"
        />
        <input
          v-model="search"
          type="search"
          class="form-input h-9 pl-8 text-xs"
          placeholder="搜索用户名、显示名称或邮箱"
          aria-label="搜索用户"
          @input="scheduleSearch"
        />
      </div>
      <p class="text-xs text-ink-muted">共 {{ total }} 个账号</p>
    </div>

    <div v-if="error" class="card border-danger/30">
      <div class="card-body flex items-center justify-between">
        <p class="text-sm text-danger">{{ error }}</p>
        <button type="button" class="btn btn-sm" @click="load">重试</button>
      </div>
    </div>

    <div class="card overflow-hidden">
      <div v-if="loading" class="divide-y divide-line">
        <div v-for="i in 5" :key="i" class="h-14 animate-pulse bg-surface-muted/60" />
      </div>
      <div v-else-if="users.length === 0" class="empty-state">
        <AppIcon name="search" :size="22" class="text-ink-subtle" />
        <p class="mt-3 text-sm font-medium text-ink">没有匹配的账号</p>
        <p class="mt-1 text-xs text-ink-muted">调整搜索关键词，或创建新的用户。</p>
      </div>
      <div v-else class="overflow-x-auto">
        <table class="w-full min-w-[680px] text-left text-sm">
          <thead class="border-b border-line bg-surface-muted/60 text-xs text-ink-subtle">
            <tr>
              <th class="px-4 py-2.5 font-medium">用户</th>
              <th class="px-4 py-2.5 font-medium">邮箱</th>
              <th class="px-4 py-2.5 font-medium">角色</th>
              <th class="px-4 py-2.5 font-medium">状态</th>
              <th class="px-4 py-2.5 font-medium">创建时间</th>
              <th class="px-4 py-2.5 text-right font-medium">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-line">
            <tr v-for="user in users" :key="user.id" class="hover:bg-surface-muted/60">
              <td class="px-4 py-3">
                <div class="flex items-center gap-2.5">
                  <span class="avatar h-7 w-7 text-[10px]">{{
                    (user.display_name || user.username).slice(0, 2)
                  }}</span>
                  <div class="min-w-0">
                    <p class="truncate font-medium text-ink">
                      {{ user.display_name || user.username }}
                      <span
                        v-if="user.id === auth.user?.id"
                        class="ml-1 text-[10px] text-ink-subtle"
                        >(你)</span
                      >
                    </p>
                    <p class="truncate font-mono text-[11px] text-ink-subtle">
                      {{ user.username }}
                    </p>
                  </div>
                </div>
              </td>
              <td class="px-4 py-3 text-xs text-ink-muted">{{ user.email || '—' }}</td>
              <td class="px-4 py-3">
                <span
                  class="badge"
                  :class="
                    user.system_root || user.platform_admin
                      ? 'border-accent/25 bg-accent-soft text-accent-strong'
                      : ''
                  "
                >
                  {{ user.system_root ? 'ROOT' : user.platform_admin ? '平台管理员' : '普通用户' }}
                </span>
              </td>
              <td class="px-4 py-3">
                <span
                  class="inline-flex items-center gap-1.5 text-xs"
                  :class="user.is_active === false ? 'text-danger' : 'text-ink-muted'"
                >
                  <span
                    class="h-1.5 w-1.5 rounded-full"
                    :class="user.is_active === false ? 'bg-danger' : 'bg-accent'"
                  />
                  {{ user.is_active === false ? '已停用' : '正常' }}
                </span>
              </td>
              <td class="px-4 py-3 text-xs text-ink-muted">
                {{ user.created_at?.slice(0, 10) ?? '—' }}
              </td>
              <td class="px-4 py-3">
                <div class="flex items-center justify-end gap-1">
                  <button type="button" class="btn btn-sm" @click="openEdit(user)">编辑</button>
                  <button type="button" class="btn btn-sm" @click="toggleActive(user)">
                    {{ user.is_active === false ? '启用' : '停用' }}
                  </button>
                  <button
                    v-if="isRoot && user.id !== auth.user?.id"
                    type="button"
                    class="btn btn-sm"
                    @click="toggleAdmin(user)"
                  >
                    {{ user.platform_admin ? '撤销管理员' : '设为管理员' }}
                  </button>
                  <button
                    type="button"
                    class="btn btn-sm"
                    aria-label="撤销会话"
                    @click="revokeSessions(user)"
                  >
                    <AppIcon name="key" :size="13" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div
        v-if="!loading && total > limit"
        class="flex items-center justify-between border-t border-line px-4 py-2.5"
      >
        <p class="text-xs text-ink-muted">第 {{ page }} / {{ pages }} 页</p>
        <div class="flex gap-2">
          <button type="button" class="btn btn-sm" :disabled="offset === 0" @click="changePage(-1)">
            上一页
          </button>
          <button
            type="button"
            class="btn btn-sm"
            :disabled="offset + limit >= total"
            @click="changePage(1)"
          >
            下一页
          </button>
        </div>
      </div>
    </div>

    <Teleport to="body">
      <div
        v-if="showCreate"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/35 px-4"
        role="dialog"
        aria-modal="true"
      >
        <form
          class="w-full max-w-md rounded-xl border border-line bg-surface shadow-xl"
          @submit.prevent="submitCreate"
        >
          <div class="flex items-center justify-between border-b border-line px-5 py-4">
            <h3 class="text-sm font-semibold text-ink">创建用户</h3>
            <button
              type="button"
              class="btn btn-ghost h-8 w-8 px-0"
              aria-label="关闭"
              @click="showCreate = false"
            >
              <AppIcon name="close" :size="15" />
            </button>
          </div>
          <div class="space-y-3 px-5 py-4">
            <div>
              <label for="admin-new-username" class="field-label">用户名</label>
              <input
                id="admin-new-username"
                v-model="createForm.username"
                required
                class="form-input mt-1"
                placeholder="alice"
              />
              <p v-if="createErrors.username" class="field-error">{{ createErrors.username }}</p>
            </div>
            <div>
              <label for="admin-new-display" class="field-label">显示名称</label>
              <input
                id="admin-new-display"
                v-model="createForm.display_name"
                required
                class="form-input mt-1"
              />
              <p v-if="createErrors.display_name" class="field-error">
                {{ createErrors.display_name }}
              </p>
            </div>
            <div>
              <label for="admin-new-email" class="field-label">邮箱（可选）</label>
              <input
                id="admin-new-email"
                v-model="createForm.email"
                type="email"
                class="form-input mt-1"
              />
              <p v-if="createErrors.email" class="field-error">{{ createErrors.email }}</p>
            </div>
            <div>
              <label for="admin-new-password" class="field-label">初始密码</label>
              <input
                id="admin-new-password"
                v-model="createForm.password"
                type="password"
                required
                class="form-input mt-1"
              />
              <p class="mt-1 text-[11px] text-ink-subtle">至少 10 个字符，且不能与用户名相同。</p>
              <p v-if="createErrors.password" class="field-error">{{ createErrors.password }}</p>
            </div>
          </div>
          <div class="flex justify-end gap-2 border-t border-line px-5 py-3">
            <button type="button" class="btn btn-sm" @click="showCreate = false">取消</button>
            <button type="submit" class="btn btn-primary btn-sm" :disabled="creating">
              {{ creating ? '创建中' : '创建账号' }}
            </button>
          </div>
        </form>
      </div>
    </Teleport>

    <Teleport to="body">
      <div
        v-if="editing"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/35 px-4"
        role="dialog"
        aria-modal="true"
      >
        <form
          class="w-full max-w-md rounded-xl border border-line bg-surface shadow-xl"
          @submit.prevent="submitEdit"
        >
          <div class="flex items-center justify-between border-b border-line px-5 py-4">
            <h3 class="text-sm font-semibold text-ink">编辑 {{ editing.username }}</h3>
            <button
              type="button"
              class="btn btn-ghost h-8 w-8 px-0"
              aria-label="关闭"
              @click="editing = null"
            >
              <AppIcon name="close" :size="15" />
            </button>
          </div>
          <div class="space-y-3 px-5 py-4">
            <div>
              <label for="admin-edit-display" class="field-label">显示名称</label>
              <input
                id="admin-edit-display"
                v-model="editForm.display_name"
                required
                class="form-input mt-1"
              />
              <p v-if="editErrors.display_name" class="field-error">
                {{ editErrors.display_name }}
              </p>
            </div>
            <div>
              <label for="admin-edit-email" class="field-label">邮箱</label>
              <input
                id="admin-edit-email"
                v-model="editForm.email"
                type="email"
                class="form-input mt-1"
              />
              <p v-if="editErrors.email" class="field-error">{{ editErrors.email }}</p>
            </div>
          </div>
          <div class="flex justify-end gap-2 border-t border-line px-5 py-3">
            <button type="button" class="btn btn-sm" @click="editing = null">取消</button>
            <button type="submit" class="btn btn-primary btn-sm" :disabled="savingEdit">
              {{ savingEdit ? '保存中' : '保存' }}
            </button>
          </div>
        </form>
      </div>
    </Teleport>
  </div>
</template>
