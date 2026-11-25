<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { ApiError } from '@/api/client'
import { createProject, listProjects, type CreateProject, type Project } from '@/api/projects'
import { getTeam, listTeamMembers, type Team, type TeamMember } from '@/api/teams'
import ProjectForm from '@/components/projects/ProjectForm.vue'
import ReviewInbox from '@/components/projects/ReviewInbox.vue'
import AlertBanner from '@/components/ui/AlertBanner.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import PageTabs from '@/components/ui/PageTabs.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
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
const leadName = computed(() => {
  const map = new Map(
    members.value.map((member) => [member.user_id, member.display_name || member.username]),
  )
  return (id: string) => map.get(id) ?? '—'
})
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
    if (route.query.create === '1' && ['owner', 'admin'].includes(t.my_role ?? '')) {
      showCreate.value = true
    }
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
  <div class="page-stack">
    <PageHeader
      eyebrow="项目"
      :title="team?.name || '项目'"
      description="把计划拆成任务，让每一步进展有据可查。"
    >
      <template #actions>
        <button class="btn" :disabled="loading || busy" @click="load">刷新</button>
        <button
          v-if="canCreate"
          class="btn btn-primary"
          :disabled="busy"
          @click="showCreate = !showCreate"
        >
          {{ showCreate ? '收起创建' : '新建项目' }}
        </button>
      </template>
    </PageHeader>

    <PageTabs
      aria-label="团队导航"
      :items="[
        { label: '项目', active: true, count: projects.length },
        { label: '成员', to: `/teams/${teamId}/members` },
        { label: '我的贡献', to: `/teams/${teamId}/contribution-results` },
        { label: '团队设置', to: '/settings' },
      ]"
    />

    <AlertBanner v-if="error" tone="danger" :message="error" />
    <div v-if="loading" class="card space-y-2 p-4">
      <div v-for="i in 3" :key="i" class="skeleton h-20" />
    </div>
    <template v-if="ready">
      <ReviewInbox :team="teamId" />
      <section v-if="showCreate && canCreate" class="card">
        <div class="card-header">
          <div>
            <h3 class="card-title">创建项目</h3>
            <p class="card-description">创建后你将成为项目负责人，并从团队中选择成员。</p>
          </div>
        </div>
        <fieldset class="card-body" :disabled="busy">
          <ProjectForm
            :key="confirmed"
            :members="members"
            :default-lead="auth.user?.id"
            @save="create"
          />
        </fieldset>
      </section>
      <div v-if="projects.length" class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
        <RouterLink
          v-for="p in projects"
          :key="p.id"
          :to="{ name: 'project-detail', params: { teamId: p.team_id, projectId: p.id } }"
          class="card group flex flex-col transition-colors hover:border-line-strong"
        >
          <div class="card-body flex-1">
            <div class="flex items-center justify-between gap-2">
              <StatusBadge
                :label="p.status === 'archived' ? '已归档 · 只读' : '进行中'"
                :tone="p.status === 'archived' ? 'neutral' : 'success'"
                dot
              />
              <span v-if="p.can_manage" class="text-meta text-accent-strong">我负责</span>
            </div>
            <h3
              class="mt-4 text-title font-semibold text-ink transition-colors group-hover:text-accent-strong"
            >
              {{ p.name }}
            </h3>
            <p class="mt-2 line-clamp-3 text-caption leading-5 text-ink-muted">
              {{ p.description || '暂无项目描述' }}
            </p>
          </div>
          <div class="card-footer text-meta text-ink-subtle">
            <span>负责人 {{ leadName(p.lead_id) }}</span>
            <span>创建于 {{ p.created_at.slice(0, 10) }}</span>
          </div>
        </RouterLink>
      </div>
      <div v-else class="card">
        <EmptyState
          icon="folder"
          title="还没有项目"
          :description="canCreate ? '创建首个项目，开始规划任务。' : '项目创建后会出现在这里。'"
        >
          <template v-if="canCreate && !showCreate" #action>
            <button type="button" class="btn btn-primary btn-sm" @click="showCreate = true">
              创建项目
            </button>
          </template>
        </EmptyState>
      </div>
    </template>
    <RouterLink to="/teams" class="link text-caption">← 我的团队</RouterLink>
  </div>
</template>
