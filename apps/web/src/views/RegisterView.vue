<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'

import { ApiError } from '@/api/client'
import { fetchMeta } from '@/api/system'
import AppIcon from '@/components/ui/AppIcon.vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()

const username = ref('')
const displayName = ref('')
const email = ref('')
const password = ref('')
const confirmPassword = ref('')
const submitting = ref(false)
const error = ref<string | null>(null)
const fieldErrors = ref<Record<string, string>>({})
const registrationMode = ref<'open' | 'invite' | 'closed'>('open')

onMounted(async () => {
  try {
    const meta = await fetchMeta()
    registrationMode.value = meta.registration_mode ?? 'open'
  } catch {
    registrationMode.value = 'open'
  }
})

function validate(): boolean {
  const fields: Record<string, string> = {}
  const name = username.value.trim().toLowerCase()
  if (!/^[a-z0-9_-]{3,32}$/.test(name)) {
    fields.username = '需为 3–32 位小写字母、数字、下划线或连字符'
  }
  if (displayName.value.trim().length > 64) fields.display_name = '不能超过 64 个字符'
  if (email.value.trim() && !/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email.value.trim())) {
    fields.email = '请输入有效的邮箱地址'
  }
  if (new TextEncoder().encode(password.value).length < 10) fields.password = '密码至少 10 个字符'
  if (password.value !== confirmPassword.value) fields.confirm_password = '两次输入的密码不一致'
  fieldErrors.value = fields
  return Object.keys(fields).length === 0
}

async function submit(): Promise<void> {
  if (submitting.value || !validate()) return
  submitting.value = true
  error.value = null
  try {
    await auth.register({
      username: username.value.trim().toLowerCase(),
      display_name: displayName.value.trim(),
      email: email.value.trim(),
      password: password.value,
    })
    await router.push({
      name: 'login',
      query: { registered: '1', username: username.value.trim().toLowerCase() },
    })
  } catch (reason) {
    if (reason instanceof ApiError) {
      error.value = reason.message
      fieldErrors.value = { ...fieldErrors.value, ...(reason.fields ?? {}) }
    } else {
      error.value = '无法连接到 Nexora API'
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="grid min-h-screen bg-canvas lg:grid-cols-[minmax(0,1fr)_minmax(420px,520px)]">
    <section
      class="relative hidden flex-col justify-between overflow-hidden border-r border-line bg-surface px-10 py-10 lg:flex"
    >
      <RouterLink to="/login" class="flex items-center gap-2.5">
        <span
          class="flex h-8 w-8 items-center justify-center rounded-lg bg-accent text-sm font-semibold text-white"
          >N</span
        >
        <span class="text-base font-semibold tracking-tight text-ink">Nexora</span>
      </RouterLink>

      <div class="max-w-md">
        <p class="eyebrow">开始协作</p>
        <h2 class="mt-3 text-3xl font-semibold leading-tight tracking-tight text-ink">
          加入团队，从第一个任务开始。
        </h2>
        <p class="mt-4 text-sm leading-7 text-ink-muted">
          注册后即可创建自己的团队，或接受管理员通过用户名发出的邀请。不同团队之间的数据相互隔离。
        </p>
        <div
          class="mt-8 rounded-xl border border-line bg-canvas p-4 text-sm leading-6 text-ink-muted"
        >
          <p class="flex items-center gap-2 font-medium text-ink">
            <AppIcon name="inbox" :size="16" />
            已被邀请？
          </p>
          <p class="mt-1.5">
            请使用管理员邀请时填写的用户名注册。注册完成后登录，邀请会出现在「邀请收件箱」中。
          </p>
        </div>
      </div>

      <p class="text-[11px] text-ink-subtle">Nexora · 本地开发环境</p>
    </section>

    <section class="flex items-center justify-center px-4 py-10 sm:px-8">
      <div class="w-full max-w-[400px]">
        <div class="mb-8 flex items-center gap-2.5 lg:hidden">
          <span
            class="flex h-8 w-8 items-center justify-center rounded-lg bg-accent text-sm font-semibold text-white"
            >N</span
          >
          <span class="text-base font-semibold tracking-tight text-ink">Nexora</span>
        </div>

        <div
          v-if="registrationMode === 'closed'"
          class="rounded-lg border border-warning/30 bg-warning-soft px-4 py-3 text-sm leading-6 text-warning"
        >
          当前环境未开放自助注册。请联系管理员创建账号。
        </div>

        <template v-else>
          <div>
            <h1 class="text-xl font-semibold tracking-tight text-ink">创建账号</h1>
            <p class="mt-1.5 text-sm text-ink-muted">
              {{
                registrationMode === 'invite'
                  ? '当前为邀请注册：请使用被邀请的用户名。'
                  : '注册后即可创建团队或接受邀请。'
              }}
            </p>
          </div>

          <form class="mt-6 space-y-4" @submit.prevent="submit">
            <div
              v-if="error"
              class="rounded-lg border border-danger/30 bg-danger-soft px-3 py-2 text-sm text-danger"
              role="alert"
            >
              {{ error }}
            </div>

            <div>
              <label for="register-username" class="field-label">用户名</label>
              <input
                id="register-username"
                v-model="username"
                type="text"
                required
                maxlength="32"
                autocomplete="username"
                class="form-input mt-1"
                placeholder="例如 alice"
              />
              <p class="mt-1 text-[11px] text-ink-subtle">
                3–32 位小写字母、数字、下划线或连字符，注册后不可修改。
              </p>
              <p v-if="fieldErrors.username" class="field-error">{{ fieldErrors.username }}</p>
            </div>

            <div>
              <label for="register-display-name" class="field-label">显示名称</label>
              <input
                id="register-display-name"
                v-model="displayName"
                type="text"
                required
                maxlength="64"
                autocomplete="name"
                class="form-input mt-1"
                placeholder="团队中展示的名字"
              />
              <p v-if="fieldErrors.display_name" class="field-error">
                {{ fieldErrors.display_name }}
              </p>
            </div>

            <div>
              <label for="register-email" class="field-label">邮箱（可选）</label>
              <input
                id="register-email"
                v-model="email"
                type="email"
                maxlength="254"
                autocomplete="email"
                class="form-input mt-1"
                placeholder="you@example.com"
              />
              <p v-if="fieldErrors.email" class="field-error">{{ fieldErrors.email }}</p>
            </div>

            <div>
              <label for="register-password" class="field-label">密码</label>
              <input
                id="register-password"
                v-model="password"
                type="password"
                required
                autocomplete="new-password"
                class="form-input mt-1"
              />
              <p class="mt-1 text-[11px] text-ink-subtle">至少 10 个字符，且不能与用户名相同。</p>
              <p v-if="fieldErrors.password" class="field-error">{{ fieldErrors.password }}</p>
            </div>

            <div>
              <label for="register-confirm" class="field-label">确认密码</label>
              <input
                id="register-confirm"
                v-model="confirmPassword"
                type="password"
                required
                autocomplete="new-password"
                class="form-input mt-1"
              />
              <p v-if="fieldErrors.confirm_password" class="field-error">
                {{ fieldErrors.confirm_password }}
              </p>
            </div>

            <button type="submit" class="btn btn-primary h-10 w-full" :disabled="submitting">
              <AppIcon v-if="submitting" name="refresh" :size="15" class="animate-spin" />
              {{ submitting ? '创建中' : '创建账号' }}
            </button>
          </form>

          <p class="mt-5 text-center text-sm text-ink-muted">
            已有账号？<RouterLink to="/login" class="link font-medium">返回登录</RouterLink>
          </p>
        </template>
      </div>
    </section>
  </div>
</template>
