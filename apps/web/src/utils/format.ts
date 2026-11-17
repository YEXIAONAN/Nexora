const dateTimeFormatter = new Intl.DateTimeFormat('zh-CN', {
  dateStyle: 'medium',
  timeStyle: 'medium',
})

export function formatDateTime(value: string | Date | null | undefined): string {
  if (!value) return '—'
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return dateTimeFormatter.format(date)
}

export function formatRelativeTime(value: Date | null): string {
  if (!value) return '尚未检查'
  const seconds = Math.max(0, Math.round((Date.now() - value.getTime()) / 1000))
  if (seconds < 5) return '刚刚'
  if (seconds < 60) return `${seconds} 秒前`
  const minutes = Math.round(seconds / 60)
  if (minutes < 60) return `${minutes} 分钟前`
  return formatDateTime(value)
}

export function teamRoleLabel(role: string | undefined): string {
  if (role === 'owner') return '所有者'
  if (role === 'admin') return '管理员'
  if (role === 'member') return '成员'
  return '—'
}

export function invitationStatusLabel(status: string): string {
  const labels: Record<string, string> = {
    pending: '待处理',
    accepted: '已接受',
    declined: '已拒绝',
    cancelled: '已取消',
  }
  return labels[status] ?? status
}
