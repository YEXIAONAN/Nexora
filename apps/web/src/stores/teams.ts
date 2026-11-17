import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { ApiError } from '@/api/client'
import {
  createTeam as apiCreateTeam,
  getTeamPermissions,
  listMyTeams,
  type Team,
  type TeamPermissions,
} from '@/api/teams'

const CURRENT_TEAM_KEY = 'nexora.currentTeam'

function readPreference(): string | null {
  try {
    return window.localStorage.getItem(CURRENT_TEAM_KEY)
  } catch {
    return null
  }
}

function writePreference(teamId: string | null): void {
  try {
    if (teamId === null) window.localStorage.removeItem(CURRENT_TEAM_KEY)
    else window.localStorage.setItem(CURRENT_TEAM_KEY, teamId)
  } catch {
    // Preference persistence is best effort; selection still works in memory.
  }
}

export const useTeamsStore = defineStore('teams', () => {
  const teams = ref<Team[]>([])
  const currentTeamId = ref<string | null>(null)
  const permissions = ref<TeamPermissions | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)
  let loadedForUser: string | null = null

  const currentTeam = computed(() => teams.value.find((t) => t.id === currentTeamId.value) ?? null)

  function errorMessage(reason: unknown): string {
    return reason instanceof ApiError ? reason.message : '无法连接到 Nexora API'
  }

  function reconcileSelection(): void {
    const ids = new Set(teams.value.map((t) => t.id))
    const preferred = currentTeamId.value ?? readPreference()
    if (preferred !== null && ids.has(preferred)) {
      currentTeamId.value = preferred
    } else {
      currentTeamId.value = teams.value[0]?.id ?? null
    }
    writePreference(currentTeamId.value)
    if (permissions.value && permissions.value.team_id !== currentTeamId.value) {
      permissions.value = null
    }
  }

  async function loadTeams(userId: string, force = false): Promise<void> {
    if (loading.value) return
    if (!force && loadedForUser === userId && teams.value.length > 0) return
    loading.value = true
    error.value = null
    try {
      teams.value = await listMyTeams()
      loadedForUser = userId
      reconcileSelection()
    } catch (reason) {
      error.value = errorMessage(reason)
      teams.value = []
      currentTeamId.value = null
      permissions.value = null
    } finally {
      loading.value = false
    }
  }

  function selectTeam(teamId: string): void {
    if (!teams.value.some((t) => t.id === teamId)) return
    currentTeamId.value = teamId
    writePreference(teamId)
    if (permissions.value && permissions.value.team_id !== teamId) permissions.value = null
  }

  async function createTeam(name: string, description?: string): Promise<Team> {
    const team = await apiCreateTeam(name, description)
    teams.value = [...teams.value, team]
    selectTeam(team.id)
    return team
  }

  async function loadPermissions(teamId: string): Promise<TeamPermissions> {
    const result = await getTeamPermissions(teamId)
    if (currentTeamId.value === teamId) permissions.value = result
    return result
  }

  function reset(): void {
    teams.value = []
    currentTeamId.value = null
    permissions.value = null
    error.value = null
    loadedForUser = null
    writePreference(null)
  }

  return {
    teams,
    currentTeamId,
    currentTeam,
    permissions,
    loading,
    error,
    loadTeams,
    selectTeam,
    createTeam,
    loadPermissions,
    reset,
  }
})
