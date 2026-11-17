<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { ApiError } from '@/api/client'
import { createProject, listProjects, type CreateProject, type Project } from '@/api/projects'
import { getTeam, listTeamMembers, type Team, type TeamMember } from '@/api/teams'
import ProjectForm from '@/components/projects/ProjectForm.vue'
import ReviewInbox from '@/components/projects/ReviewInbox.vue'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
const route = useRoute(),
  router = useRouter(),
  teams = useTeamsStore(),
  auth = useAuthStore()
const teamId = computed(() => String(route.params.teamId ?? ''))
const projects = ref<Project[]>([]),
  team = ref<Team | null>(null),
  members = ref<TeamMember[]>([])
const loading = ref(false),
  busy = ref(false),
  error = ref(''),
  showCreate = ref(false),
  confirmed = ref('')
let generation = 0
const ready = computed(() => confirmed.value === teamId.value && !!team.value && !loading.value)
const canCreate = computed(
  () => ready.value && ['owner', 'admin'].includes(team.value?.my_role ?? ''),
)
async function load() {
  const id = teamId.value,
    gen = ++generation
  confirmed.value = ''
  loading.value = true
  error.value = ''
  projects.value = []
  members.value = []
  team.value = null
  showCreate.value = false
  try {
    const [t, p, m] = await Promise.all([getTeam(id), listProjects(id), listTeamMembers(id)])
    if (generation !== gen || id !== teamId.value) return
    team.value = t
    projects.value = p
    members.value = m
    confirmed.value = id
    teams.selectTeam(id)
  } catch (e) {
    if (generation === gen && id === teamId.value)
      error.value = e instanceof ApiError ? e.message : '项目加载失败'
  } finally {
    if (generation === gen && id === teamId.value) loading.value = false
  }
}
async function create(input: CreateProject) {
  if (!ready.value || busy.value) return
  const id = confirmed.value,
    gen = generation
  busy.value = true
  error.value = ''
  try {
    const project = await createProject(id, input)
    if (gen !== generation || id !== teamId.value) return
    await router.push({ name: 'project-detail', params: { teamId: id, projectId: project.id } })
  } catch (e) {
    if (gen === generation && id === teamId.value)
      error.value = e instanceof ApiError ? e.message : '创建失败'
  } finally {
    if (gen === generation && id === teamId.value) busy.value = false
  }
}
watch(
  teamId,
  () => {
    busy.value = false
    void load()
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  generation++
})
</script>
<template>
  <div class="space-y-6">
    <div class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <p class="text-xs text-ink-subtle">{{ team?.name || '团队工作区' }}</p>
        <h2 class="mt-1 text-2xl font-semibold">项目</h2>
        <p class="mt-2 text-sm text-ink-muted">把计划拆成任务，让每一步进展有据可查。</p>
      </div>
      <div class="flex gap-2">
        <button class="btn" :disabled="loading || busy" @click="load">刷新</button
        ><button
          v-if="canCreate"
          class="btn btn-primary"
          :disabled="busy"
          @click="showCreate = !showCreate"
        >
          新建项目
        </button>
      </div>
    </div>
    <p v-if="error" role="alert" class="rounded-lg bg-danger-soft p-4 text-sm text-danger">
      {{ error }}
    </p>
    <p v-if="loading" role="status" class="card p-8 text-sm text-ink-muted">正在加载项目…</p>
    <template v-if="ready">
      <ReviewInbox :team="teamId" />
      <section v-if="showCreate && canCreate" class="card">
        <div class="card-header"><h3 class="font-semibold">创建项目</h3></div>
        <fieldset class="card-body" :disabled="busy">
          <ProjectForm
            :key="confirmed"
            :members="members"
            :default-lead="auth.user?.id"
            @save="create"
          />
        </fieldset>
      </section>
      <div v-if="projects.length" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        <RouterLink
          v-for="p in projects"
          :key="p.id"
          :to="{ name: 'project-detail', params: { teamId: p.team_id, projectId: p.id } }"
          class="card group p-5 transition hover:border-accent"
          ><span class="badge">{{ p.status === 'archived' ? '已归档 · 只读' : '进行中' }}</span>
          <h3 class="mt-4 text-lg font-semibold group-hover:text-accent-strong">{{ p.name }}</h3>
          <p class="mt-2 line-clamp-3 text-sm leading-6 text-ink-muted">
            {{ p.description || '暂无项目描述' }}
          </p>
          <p class="mt-5 text-xs text-ink-subtle">
            {{ p.can_manage ? '我负责的项目' : '团队项目'
            }}<span class="float-right text-accent-strong">查看项目 →</span>
          </p></RouterLink
        >
      </div>
      <div v-else class="card p-12 text-center">
        <h3 class="font-medium">还没有项目</h3>
        <p class="mt-2 text-sm text-ink-muted">
          {{ canCreate ? '创建首个项目，开始规划任务。' : '项目创建后会出现在这里。' }}
        </p>
      </div>
    </template>
    <RouterLink to="/teams" class="inline-block text-sm text-accent-strong">← 我的团队</RouterLink>
  </div>
</template>
