<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import type { BonusMember, CreateBonusRound } from '@/api/bonus'

const props = defineProps<{ members: BonusMember[]; userId: string; busy: boolean }>()
const emit = defineEmits<{ create: [input: Omit<CreateBonusRound, 'client_request_id'>] }>()
const fields = reactive({
  funding_key: '',
  funding_label: '',
  funding_source: '',
  manager_id: '',
  reviewer_id: '',
  reason: '',
})
const error = ref('')
const candidates = computed(() => props.members.filter((m) => m.user_id !== props.userId))
function submit() {
  if (props.busy) return
  error.value = ''
  if (!props.userId) {
    error.value = '当前身份不可用，请刷新后重试。'
    return
  }
  if (!/^[a-z0-9_-]+$/.test(fields.funding_key.trim())) {
    error.value = '来源标识只能使用小写 a-z、数字、连字符和下划线。'
    return
  }
  if (!fields.funding_label.trim() || !fields.funding_source.trim() || !fields.reason.trim()) {
    error.value = '请填写批次名称、真实资金来源与创建理由。'
    return
  }
  if (
    !candidates.value.some((m) => m.user_id === fields.manager_id) ||
    !candidates.value.some((m) => m.user_id === fields.reviewer_id)
  ) {
    error.value = '请选择其他活跃项目成员；不可自我授权。'
    return
  }
  if (fields.manager_id === fields.reviewer_id) {
    error.value = '金额负责人和独立核定人必须是不同成员。'
    return
  }
  emit('create', {
    ...fields,
    funding_key: fields.funding_key.trim(),
    funding_label: fields.funding_label.trim(),
    funding_source: fields.funding_source.trim(),
    reason: fields.reason.trim(),
  })
}
</script>

<template>
  <section class="card">
    <div class="card-header">
      <div>
        <h3 class="section-title">创建独立资金批次</h3>
        <p class="mt-1 text-xs leading-5 text-ink-muted">
          每个标识代表一笔独立真实资金，不用于复制同一资金。初始预算为 0，由选定金额负责人后续填写。
        </p>
      </div>
    </div>
    <form class="card-body" @submit.prevent="submit">
      <fieldset :disabled="busy" class="space-y-4">
        <div class="grid gap-4 sm:grid-cols-2">
          <label class="form-label"
            >来源标识<input
              v-model="fields.funding_key"
              class="form-input"
              required
              placeholder="例如 school-award-2026"
          /></label>
          <label class="form-label"
            >批次名称<input v-model="fields.funding_label" class="form-input" required
          /></label>
        </div>
        <label class="form-label"
          >真实资金来源<textarea
            v-model="fields.funding_source"
            class="form-input"
            rows="2"
            required
            placeholder="说明资金提供方、来源与独立性"
          />
        </label>
        <div class="grid gap-4 sm:grid-cols-2">
          <label class="form-label"
            >金额负责人<select v-model="fields.manager_id" class="form-input" required>
              <option value="">请选择成员</option>
              <option v-for="m in candidates" :key="m.user_id" :value="m.user_id">
                {{ m.display_name }}
              </option>
            </select></label
          >
          <label class="form-label"
            >独立核定人<select v-model="fields.reviewer_id" class="form-input" required>
              <option value="">请选择另一位成员</option>
              <option
                v-for="m in candidates.filter((m) => m.user_id !== fields.manager_id)"
                :key="m.user_id"
                :value="m.user_id"
              >
                {{ m.display_name }}
              </option>
            </select></label
          >
        </div>
        <p class="text-xs leading-5 text-ink-muted">
          不可自我授权：负责人、核定人互不相同，且均不能是创建者。创建者不会自动获得金额查看或编辑权；核定人还需满足利益冲突检查。
        </p>
        <p v-if="candidates.length < 2" class="text-xs text-danger">
          至少需要两位其他活跃项目成员，请先在项目工作区补齐成员。
        </p>
        <label class="form-label"
          >创建理由<textarea v-model="fields.reason" class="form-input" rows="2" required />
        </label>
        <p v-if="error" role="alert" class="text-sm text-danger">{{ error }}</p>
        <button type="submit" class="btn btn-primary">{{ busy ? '正在创建…' : '创建批次' }}</button>
      </fieldset>
    </form>
  </section>
</template>
