import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import StatusBadge from './StatusBadge.vue'

describe('StatusBadge', () => {
  it('renders the label and the requested tone', () => {
    const wrapper = mount(StatusBadge, {
      props: { label: '正常', tone: 'success', dot: true },
    })

    expect(wrapper.text()).toContain('正常')
    expect(wrapper.classes()).toContain('bg-accent-soft')
    expect(wrapper.find('span span').exists()).toBe(true)
  })

  it('defaults to the neutral tone without a dot', () => {
    const wrapper = mount(StatusBadge, { props: { label: '未知' } })

    expect(wrapper.classes()).toContain('bg-surface-muted')
    expect(wrapper.find('span span').exists()).toBe(false)
  })
})
