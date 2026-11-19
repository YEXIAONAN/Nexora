<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type {
  ContributionDraftRequest,
  ContributionShare,
  ContributionShareRequest,
  ContributionVersion,
} from '@/api/contributions'
import type { ProjectMember } from '@/api/projects'
import { percentToBasisPoints, formatBasisPoints } from './contributionPresentation'
const props = defineProps<{
  version: ContributionVersion
  members: ProjectMember[]
  busy: boolean
}>()
const emit = defineEmits<{
  save: [input: Pick<ContributionDraftRequest, 'summary' | 'evidence_snapshot' | 'reason'>]
  share: [
    user: string,
    input: Pick<
      ContributionShareRequest,
      'share_bps' | 'basis' | 'statement' | 'evidence_refs' | 'reason'
    >,
  ]
  remove: [user: string, reason: string]
}>()
const summary = ref(props.version.summary),
  notes = ref(
    typeof props.version.evidence_snapshot.notes === 'string'
      ? props.version.evidence_snapshot.notes
      : '',
  ),
  reason = ref('')
const user = ref(''),
  percentage = ref(''),
  basis = ref(''),
  statement = ref(''),
  shareReason = ref('')
const refs = ref<ContributionShare['evidence_refs']>([])
const bps = computed(() => {
  const value = percentToBasisPoints(percentage.value)
  return value !== null && value > 0 ? value : null
})
const refsValid = computed(() => refs.value.every((r) => r.source.trim() && r.reference.trim()))
function selectShare(share: ContributionShare) {
  user.value = share.user_id
  percentage.value = formatBasisPoints(share.share_bps).slice(0, -1)
  basis.value = share.basis
  statement.value = share.statement
  refs.value = share.evidence_refs.map((r) => ({ ...r }))
  shareReason.value = ''
}
function resetShare() {
  user.value = ''
  percentage.value = ''
  basis.value = ''
  statement.value = ''
  shareReason.value = ''
  refs.value = []
}
watch(
  () => props.version,
  () => {
    summary.value = props.version.summary
    notes.value =
      typeof props.version.evidence_snapshot.notes === 'string'
        ? props.version.evidence_snapshot.notes
        : ''
    reason.value = ''
    resetShare()
  },
)
function saveShare() {
  if (
    !props.busy &&
    user.value &&
    bps.value !== null &&
    shareReason.value.trim() &&
    refsValid.value
  )
    emit('share', user.value, {
      share_bps: bps.value,
      basis: basis.value,
      statement: statement.value,
      evidence_refs: refs.value.map((r) => ({ ...r })),
      reason: shareReason.value,
    })
}
</script>
<template>
  <section class="card" aria-label="贡献草案编辑">
    <div class="card-header">
      <h2 class="font-semibold">编辑草案 V{{ version.version }}</h2>
    </div>
    <fieldset :disabled="busy" class="card-body space-y-5">
      <p class="rounded-lg bg-warning-soft p-3 text-sm text-warning">
        草案不是正式贡献。份额由人工填写，可暂不满 100%；提交必须合计恰好 10000 基点。
      </p>
      <form
        class="space-y-3"
        @submit.prevent="
          emit('save', {
            summary,
            evidence_snapshot: { ...version.evidence_snapshot, notes },
            reason,
          })
        "
      >
        <label class="form-label"
          >草案说明<textarea v-model="summary" maxlength="8000" rows="4" class="form-input" />
        </label>
        <label class="form-label"
          >证据整理说明<textarea v-model="notes" maxlength="8000" rows="3" class="form-input" />
        </label>
        <details
          v-if="Object.keys(version.evidence_snapshot).some((k) => k !== 'notes')"
          class="text-xs text-ink-muted"
        >
          <summary>查看已有证据快照</summary>
          <pre class="overflow-auto whitespace-pre-wrap">{{
            JSON.stringify(version.evidence_snapshot, null, 2)
          }}</pre>
        </details>
        <label class="form-label"
          >修改原因<input v-model="reason" required maxlength="1000" class="form-input"
        /></label>
        <button type="submit" class="btn" :disabled="!reason.trim()">保存草案说明</button>
      </form>
      <div class="border-t border-line pt-4">
        <h3 class="text-sm font-semibold">草案份额</h3>
        <p class="mt-2 text-xs text-ink-muted">
          当前合计 {{ version.total_bps }} / 10000
          基点。贡献依据、陈述和证据引用是编辑者整理的材料，不等于本人签署或已核验事实。
        </p>
        <ul class="mt-3 space-y-2">
          <li
            v-for="share in version.shares"
            :key="share.user_id"
            class="flex flex-wrap items-center gap-2 rounded-lg bg-surface-muted p-3 text-sm"
          >
            <span class="grow"
              >{{
                members.find((m) => m.user_id === share.user_id)?.display_name ||
                members.find((m) => m.user_id === share.user_id)?.username ||
                share.user_id
              }}
              · {{ share.share_bps }} 基点</span
            ><button type="button" class="btn" @click="selectShare(share)">编辑此份额</button
            ><button
              type="button"
              class="btn text-danger"
              :disabled="!shareReason.trim()"
              @click="emit('remove', share.user_id, shareReason)"
            >
              删除此份额
            </button>
          </li>
        </ul>
      </div>
      <form class="space-y-3" @submit.prevent="saveShare">
        <h3 class="text-sm font-semibold">添加或修改贡献者</h3>
        <label class="form-label"
          >贡献者<select v-model="user" required class="form-input">
            <option value="">请选择真实项目成员</option>
            <option v-for="m in members" :key="m.user_id" :value="m.user_id">
              {{ m.display_name || m.username }}{{ m.active ? '' : '（历史成员）' }}
            </option>
            <option
              v-for="share in version.shares.filter(
                (s) => !members.some((m) => m.user_id === s.user_id),
              )"
              :key="share.user_id"
              :value="share.user_id"
            >
              {{ share.user_id }}（已有贡献者）
            </option>
          </select></label
        >
        <label class="form-label"
          >贡献份额（%）<input
            v-model="percentage"
            required
            inputmode="decimal"
            class="form-input"
            placeholder="例如 33.33"
            aria-describedby="share-precision"
        /></label>
        <p id="share-precision" class="text-xs text-ink-muted">
          最多两位小数；{{
            bps === null ? '请输入 0.01 至 100.00' : `将发送 ${bps} 个整数基点`
          }}。不自动补齐或分配剩余份额。
        </p>
        <label class="form-label"
          >贡献依据<textarea v-model="basis" maxlength="4000" rows="3" class="form-input" />
        </label>
        <label class="form-label"
          >贡献说明<textarea v-model="statement" maxlength="8000" rows="3" class="form-input" />
        </label>
        <div v-for="(e, i) in refs" :key="i" class="space-y-2 rounded-lg border border-line p-3">
          <label class="form-label"
            >引用分类<select v-model="e.kind" class="form-input">
              <option value="objective_fact">客观事实</option>
              <option value="self_statement">本人陈述</option>
              <option value="third_party">第三方佐证</option>
            </select></label
          >
          <label class="form-label"
            >证据来源<input v-model="e.source" required maxlength="1000" class="form-input"
          /></label>
          <label class="form-label"
            >证据引用<textarea v-model="e.reference" required maxlength="2000" class="form-input" />
          </label>
          <button type="button" class="btn btn-ghost" @click="refs.splice(i, 1)">
            移除此证据引用
          </button>
        </div>
        <button
          type="button"
          class="btn"
          :disabled="refs.length >= 50"
          @click="refs.push({ kind: 'objective_fact', source: '', reference: '' })"
        >
          添加贡献证据引用
        </button>
        <label class="form-label"
          >份额修改原因<input v-model="shareReason" required maxlength="1000" class="form-input"
        /></label>
        <p class="text-xs text-ink-muted">
          删除份额也须先填写此原因。受益人不得整理自己的案例；服务端将实时校验成员资格与利益冲突。
        </p>
        <div class="flex flex-wrap gap-2">
          <button
            type="submit"
            class="btn btn-primary"
            :disabled="!user || bps === null || !shareReason.trim() || !refsValid"
          >
            保存贡献份额</button
          ><button type="button" class="btn btn-ghost" @click="resetShare">清空份额表单</button>
        </div>
      </form>
    </fieldset>
  </section>
</template>
