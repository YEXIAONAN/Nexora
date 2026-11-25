<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'

import { ApiError } from '@/api/client'
import { fetchMeta } from '@/api/system'
import AppIcon from '@/components/ui/AppIcon.vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const username = ref(typeof route.query.username === 'string' ? route.query.username : '')
const password = ref('')
const submitting = ref(false)
const error = ref<string | null>(null)
const fieldErrors = ref<Record<string, string>>({})
const rootUninitialized = ref(false)
const registrationMode = ref('open')
const expired = ref(route.query.expired === '1')
const registered = ref(route.query.registered === '1')

onMounted(async () => {
  try {
    const meta = await fetchMeta()
    rootUninitialized.value = meta.root_initialized === false
    registrationMode.value = meta.registration_mode ?? 'open'
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
        <p class="eyebrow">团队任务与贡献协作</p>
        <h2 class="mt-3 text-3xl font-semibold leading-tight tracking-tight text-ink">
          让每一项交付都清晰、可追溯。
        </h2>
        <p class="mt-4 text-sm leading-7 text-ink-muted">
          在一个工作空间里管理团队、项目与三级任务，通过成果提交和独立审核沉淀可信的贡献记录。
        </p>
        <ul class="mt-8 space-y-3 text-sm text-ink-muted">
          <li class="flex items-center gap-2.5">
            <AppIcon name="check" :size="16" class="text-accent-strong" />
            多团队隔离与独立角色
          </li>
          <li class="flex items-center gap-2.5">
            <AppIcon name="check" :size="16" class="text-accent-strong" />
            项目、任务与成果审核闭环
          </li>
          <li class="flex items-center gap-2.5">
            <AppIcon name="check" :size="16" class="text-accent-strong" />
            可追溯的贡献与审核记录
          </li>
        </ul>
      </div>

      <p class="text-meta text-ink-subtle">Nexora Alpha · 团队内部试用</p>
    </section>

    <section class="flex items-center justify-center px-4 py-10 sm:px-8">
      <div class="w-full max-w-[380px]">
        <div class="mb-8 flex items-center gap-2.5 lg:hidden">
          <span
            class="flex h-8 w-8 items-center justify-center rounded-lg bg-accent text-sm font-semibold text-white"
            >N</span
          >
          <span class="text-base font-semibold tracking-tight text-ink">Nexora</span>
        </div>

        <div
          v-if="expired"
          class="mb-4 rounded-lg border border-warning/30 bg-warning-soft px-4 py-3 text-sm text-warning"
          role="status"
        >
          会话已过期或已被撤销，请重新登录。
        </div>
        <div
          v-if="registered"
          class="mb-4 rounded-lg border border-accent/30 bg-accent-soft px-4 py-3 text-sm text-accent-strong"
          role="status"
        >
          账号已创建，请使用新账号登录。
        </div>
        <div
          v-if="rootUninitialized"
          class="mb-4 rounded-lg border border-warning/30 bg-warning-soft px-4 py-3 text-sm leading-6 text-warning"
          role="status"
        >
          <p class="font-medium">系统尚未初始化 ROOT 账号</p>
          <p class="mt-1">
            运行 <code class="font-mono text-xs">./start</code> 后按提示设置管理员密码，或执行
            <code class="font-mono text-xs">make bootstrap</code>。系统不提供默认密码。
          </p>
        </div>

        <div>
          <h1 class="text-xl font-semibold tracking-tight text-ink">登录</h1>
          <p class="mt-1.5 text-sm text-ink-muted">使用你的 Nexora 账号进入工作台。</p>
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
            <label for="login-username" class="field-label">用户名</label>
            <input
              id="login-username"
              v-model="username"
              type="text"
              name="username"
              autocomplete="username"
              required
              class="form-input mt-1"
              :aria-invalid="Boolean(fieldErrors.username)"
            />
            <p v-if="fieldErrors.username" class="field-error">{{ fieldErrors.username }}</p>
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
              class="form-input mt-1"
              :aria-invalid="Boolean(fieldErrors.password)"
            />
            <p v-if="fieldErrors.password" class="field-error">{{ fieldErrors.password }}</p>
          </div>

          <button type="submit" class="btn btn-primary h-10 w-full" :disabled="submitting">
            <AppIcon v-if="submitting" name="refresh" :size="15" class="animate-spin" />
            {{ submitting ? '登录中' : '登录' }}
          </button>
        </form>

        <p v-if="registrationMode !== 'closed'" class="mt-5 text-center text-sm text-ink-muted">
          还没有账号？
          <RouterLink to="/register" class="link font-medium">创建账号</RouterLink>
        </p>
        <p v-else class="mt-5 text-center text-xs text-ink-subtle">
          本环境未开放自助注册，请联系管理员创建账号。
        </p>

        <p class="mt-6 text-center text-meta text-ink-subtle">
          会话通过 HttpOnly Cookie 维持，不存储在浏览器本地存储中。
        </p>
      </div>
    </section>
  </div>
</template>
