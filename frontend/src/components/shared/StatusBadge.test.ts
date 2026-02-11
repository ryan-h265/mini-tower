import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import StatusBadge from './StatusBadge.vue'

describe('StatusBadge', () => {
  it('renders running status with info class', () => {
    const wrapper = mount(StatusBadge, {
      props: { status: 'running' }
    })

    expect(wrapper.text()).toBe('running')
    expect(wrapper.classes()).toContain('info')
  })

  it('renders failed status with error class', () => {
    const wrapper = mount(StatusBadge, {
      props: { status: 'failed' }
    })

    expect(wrapper.text()).toBe('failed')
    expect(wrapper.classes()).toContain('error')
  })
})
