import { vi, describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import OrderListView from '../OrderListView.vue'
import { signedFetch } from '../../api/request'
import { createPinia, setActivePinia } from 'pinia'

// Mock vue-router
const mockPush = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: mockPush
  })
}))

import { useAuthStore } from '../../stores/auth'

// Mock request
vi.mock('../../api/request', () => ({
  signedFetch: vi.fn()
}))

describe('OrderListView.vue', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.stubGlobal('alert', vi.fn())
    vi.stubGlobal('localStorage', {
      getItem: vi.fn().mockReturnValue(null),
      setItem: vi.fn(),
      removeItem: vi.fn(),
      clear: vi.fn(),
    })
    setActivePinia(createPinia())
    // 默认登出，为普通用户状态
    useAuthStore().logout()
  })

  it('renders empty state when there are no orders', async () => {
    // Mock 接口返回空订单与基本监控数据
    vi.mocked(signedFetch).mockImplementation((url) => {
      if (String(url).includes('/api/metrics')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({
            metrics: { seckillStock: 10, lockStock: 0, ordersPaid: 0, revenue: '0.00' },
            logs: []
          })
        } as any)
      }
      if (String(url).includes('/api/orders')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve([])
        } as any)
      }
      return Promise.reject(new Error('Unknown url'))
    })

    const wrapper = mount(OrderListView)
    
    // 等待异步数据加载
    await new Promise(resolve => setTimeout(resolve, 50))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('您目前没有任何秒杀或普通订单')
  })

  it('renders order list cards when orders exist', async () => {
    const mockOrders = [
      {
        id: 'order_999',
        createdAt: new Date().toISOString(),
        totalAmount: 19900,
        status: 20, // 已支付
        items: [
          {
            id: 1,
            skuId: 1001,
            price: 19900,
            quantity: 1,
            sku: { title: '测试手机', specs: JSON.stringify({ 规格: '红色 128G' }) }
          }
        ]
      }
    ]

    vi.mocked(signedFetch).mockImplementation((url) => {
      if (String(url).includes('/api/metrics')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({
            metrics: { seckillStock: 10, lockStock: 0, ordersPaid: 1, revenue: '199.00' }
          })
        } as any)
      }
      if (String(url).includes('/api/orders')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(mockOrders)
        } as any)
      }
      return Promise.reject(new Error('Unknown url'))
    })

    const wrapper = mount(OrderListView)
    await new Promise(resolve => setTimeout(resolve, 50))
    await wrapper.vm.$nextTick()

    // 验证订单列表已被渲染
    expect(wrapper.text()).not.toContain('您目前没有任何秒杀或普通订单')
    expect(wrapper.text()).toContain('order_999')
    expect(wrapper.text()).toContain('测试手机')
    expect(wrapper.text()).toContain('¥199.00')
  })

  it('allows user to submit refund request', async () => {
    const mockOrders = [
      {
        id: 'order_refund_123',
        createdAt: new Date().toISOString(),
        totalAmount: 9900,
        status: 20, // 已支付，可申请退款
        items: [
          {
            id: 2,
            skuId: 1002,
            price: 9900,
            quantity: 1,
            sku: { title: '测试衣服', specs: JSON.stringify({ 规格: 'XL' }) }
          }
        ]
      }
    ]

    vi.mocked(signedFetch).mockImplementation((url, init) => {
      if (String(url).includes('/api/metrics')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ metrics: {} })
        } as any)
      }
      if (String(url).includes('/api/orders') && (!init || init.method !== 'POST')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(mockOrders)
        } as any)
      }
      // Mock 退款申请提交接口
      if (String(url).includes('/api/orders/order_refund_123/refund') && init?.method === 'POST') {
        return Promise.resolve({
          ok: true
        } as any)
      }
      return Promise.reject(new Error('Unknown url'))
    })

    const wrapper = mount(OrderListView)
    await new Promise(resolve => setTimeout(resolve, 50))
    await wrapper.vm.$nextTick()

    // 点击申请退款按钮，打开模态框
    const refundBtn = wrapper.findAll('button').find(btn => btn.text().includes('申请退款'))
    expect(refundBtn).toBeDefined()
    await refundBtn!.trigger('click')

    // 模态框打开，校验并提交
    expect((wrapper.vm as any).showRefundDialog).toBe(true)
    
    // 触发 submitRefund
    await (wrapper.vm as any).submitRefund()

    // 确认关闭模态框并发出请求
    expect((wrapper.vm as any).showRefundDialog).toBe(false)
    expect(signedFetch).toHaveBeenCalledWith(
      '/api/orders/order_refund_123/refund',
      expect.objectContaining({ method: 'POST' })
    )
  })

  it('renders merchant audit buttons and handles audit action when user is Admin', async () => {
    // 设为管理员
    useAuthStore().login('admin_user', 'mock-token', 'admin')

    const mockOrders = [
      {
        id: 'order_audit_555',
        createdAt: new Date().toISOString(),
        totalAmount: 5000,
        status: 110, // 退款申请中
        items: [
          {
            id: 3,
            skuId: 1003,
            price: 5000,
            quantity: 1,
            sku: { title: '测试鞋子', specs: JSON.stringify({ 规格: '42' }) }
          }
        ]
      }
    ]

    vi.mocked(signedFetch).mockImplementation((url, init) => {
      if (String(url).includes('/api/metrics')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ metrics: {} })
        } as any)
      }
      if (String(url).includes('/api/orders') && (!init || init.method !== 'POST')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(mockOrders)
        } as any)
      }
      if (String(url).includes('/api/admin/orders/order_audit_555/refund/audit') && init?.method === 'POST') {
        return Promise.resolve({
          ok: true
        } as any)
      }
      return Promise.reject(new Error('Unknown url'))
    })

    const wrapper = mount(OrderListView)
    await new Promise(resolve => setTimeout(resolve, 50))
    await wrapper.vm.$nextTick()

    // 应该渲染商家审核模块和“批准”/“拒绝”按钮
    expect(wrapper.text()).toContain('商家退款审核台')
    const approveBtn = wrapper.findAll('button').find(btn => btn.text().includes('批准'))
    expect(approveBtn).toBeDefined()

    // 触发同意退款
    await approveBtn!.trigger('click')
    expect(signedFetch).toHaveBeenCalledWith(
      '/api/admin/orders/order_audit_555/refund/audit',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ action: 'approve' })
      })
    )
  })
})
