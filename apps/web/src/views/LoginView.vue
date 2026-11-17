<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { ApiError } from '@/api/client'
import { fetchMeta } from '@/api/system'
import AppIcon from '@/components/ui/AppIcon.vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const username = ref('')
const password = ref('')
const submitting = ref(false)
const error = ref<string | null>(null)
const fieldErrors = ref<Record<string, string>>({})
const rootUninitialized = ref(false)
const expired = ref(route.query.expired === '1')

onMounted(async () => {
  try {
    const meta = await fetchMeta()
    rootUninitialized.value = meta.root_initialized === false
  } catch {
    rootUninitialized.value = false
  }
})

async function submit(): Promise<void> {
  if (submitting.value) return
  submitting.value = true
  error.value = null
  fieldErrors.value = {}
  expired.value = false
  try {
    await auth.login(username.value.trim(), password.value)
    const redirect = route.query.redirect
    const target =
      typeof redirect === 'string' && redirect.startsWith('/') && !redirect.startsWith('//')
        ? redirect
        : '/teams'
    await router.push(target)
  } catch (reason) {
    if (reason instanceof ApiError) {
      error.value = reason.message
      fieldErrors.value = reason.fields ?? {}
    } else {
      error.value = '无法连接到 Nexora API'
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="flex min-h-screen items-center justify-center bg-canvas px-4 py-10">
    <div class="w-full max-w-sm">
      <div class="mb-6 flex items-center justify-center gap-2.5">
        <span
          class="flex h-9 w-9 items-center justify-center rounded-lg bg-accent text-base font-semibold text-white"
          aria-hidden="true"
        >
          N
        </span>
        <span class="text-lg font-semibold tracking-tight text-ink">Nexora</span>
      </div>

      <div
        v-if="expired"
        class="mb-4 rounded-xl border border-warning/30 bg-warning-soft px-4 py-3 text-sm text-warning"
        role="status"
      >
        会话已过期或已被撤销，请重新登录。
      </div>

      <div
        v-if="rootUninitialized"
        class="mb-4 rounded-xl border border-warning/30 bg-warning-soft px-4 py-3 text-sm leading-6 text-warning"
        role="status"
      >
        <p class="font-medium">系统尚未初始化 ROOT 账号</p>
        <p class="mt-1">
          请在服务器上运行 <code class="font-mono text-xs">make bootstrap</code>（或
          <code class="font-mono text-xs">nexora-bootstrap</code>）交互式设置 ROOT
          凭据。系统不提供默认密码，也不开放公众注册。
        </p>
      </div>

      <form class="card card-body space-y-4" @submit.prevent="submit">
        <div>
          <h1 class="text-base font-semibold text-ink">登录</h1>
          <p class="mt-1 text-xs text-ink-muted">使用管理员分配的账号登录工作台。</p>
        </div>

        <div
          v-if="error"
          class="rounded-lg border border-danger/30 bg-danger-soft px-3 py-2 text-sm text-danger"
          role="alert"
        >
          {{ error }}
        </div>

        <div>
          <label for="login-username" class="field-label">用户名</label>
          <input
            id="login-username"
            v-model="username"
            type="text"
            name="username"
            autocomplete="username"
            required
            class="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink focus:border-accent focus:outline-none"
            :aria-invalid="Boolean(fieldErrors.username)"
          />
          <p v-if="fieldErrors.username" class="mt-1 text-xs text-danger">
            {{ fieldErrors.username }}
          </p>
        </div>

        <div>
          <label for="login-password" class="field-label">密码</label>
          <input
            id="login-password"
            v-model="password"
            type="password"
            name="password"
            autocomplete="current-password"
            required
            class="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink focus:border-accent focus:outline-none"
            :aria-invalid="Boolean(fieldErrors.password)"
          />
          <p v-if="fieldErrors.password" class="mt-1 text-xs text-danger">
            {{ fieldErrors.password }}
          </p>
        </div>

        <button type="submit" class="btn btn-primary w-full" :disabled="submitting">
          <AppIcon v-if="submitting" name="refresh" :size="15" class="animate-spin" />
          {{ submitting ? '登录中' : '登录' }}
        </button>
      </form>

      <p class="mt-4 text-center text-xs text-ink-subtle">
        会话通过 HttpOnly Cookie 维持，不会存储在浏览器本地存储中。
      </p>
    </div>
  </div>
</template>
