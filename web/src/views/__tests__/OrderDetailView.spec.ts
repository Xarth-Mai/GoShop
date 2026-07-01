import { vi, describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import OrderDetailView from '../OrderDetailView.vue'
import { signedFetch } from '../../api/request'
import { createPinia, setActivePinia } from 'pinia'

// Mock vue-router
const mockPush = vi.fn()
vi.mock('vue-router', () => ({
  useRoute: () => ({
    params: { id: 'order_det_001' }
  }),
  useRouter: () => ({
    push: mockPush
  })
}))

// Mock request
vi.mock('../../api/request', () => ({
  signedFetch: vi.fn()
}))

describe('OrderDetailView.vue', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.stubGlobal('localStorage', {
      getItem: vi.fn().mockReturnValue(null),
      setItem: vi.fn(),
      removeItem: vi.fn(),
      clear: vi.fn(),
    })
    setActivePinia(createPinia())
  })

  it('renders loading state initially', async () => {
    // 阻止立即 resolve，维持 loading 状态
    vi.mocked(signedFetch).mockImplementation(() => new Promise(() => {}))

    const wrapper = mount(OrderDetailView)
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('正在加载订单详情...')
  })

  it('renders not found when order does not exist', async () => {
    vi.mocked(signedFetch).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(null)
    } as any)

    const wrapper = mount(OrderDetailView)
    await new Promise(resolve => setTimeout(resolve, 50))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('订单不存在或无权访问')
  })

  it('renders order detail elements correctly when loaded', async () => {
    const mockDetail = {
      order: {
        id: 'order_det_001',
        status: 20, // 已支付
        payExpireAt: null,
        receiverName: '张三',
        receiverPhone: '13800000000',
        receiverAddr: '北京朝阳区',
        goodsOriginAmount: 20000,
        goodsDiscountAmount: 2000,
        shippingFee: 1000,
        taxFee: 500,
        totalAmount: 19500,
        items: [
          {
            id: 10,
            skuId: 5001,
            price: 20000,
            quantity: 1,
            originAmount: 20000,
            itemDiscountAmount: 2000,
            sku: { title: '高配主板', specs: JSON.stringify({ 规格: 'ATX' }) }
          }
        ]
      },
      paymentOrder: {
        id: 'pay_001',
        status: 'PAID',
        channelTradeNo: 'wx_123456789'
      },
      stateLogs: [
        { id: 101, event: 'ORDER_CREATE', remark: '订单已创建', createdAt: '2026-07-01T12:00:00Z' }
      ],
      afterSales: [],
      refundOrders: [],
      reservations: [{ id: 901 }]
    }

    vi.mocked(signedFetch).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockDetail)
    } as any)

    const wrapper = mount(OrderDetailView)
    await new Promise(resolve => setTimeout(resolve, 50))
    await wrapper.vm.$nextTick()

    // 检查页面核心信息渲染
    expect(wrapper.text()).toContain('order_det_001')
    expect(wrapper.text()).toContain('已支付')
    expect(wrapper.text()).toContain('张三 13800000000')
    expect(wrapper.text()).toContain('高配主板')
    expect(wrapper.text()).toContain('¥195.00') // 应付 195.00 元
    expect(wrapper.text()).toContain('wx_123456789') // 支付流水
    expect(wrapper.text()).toContain('ORDER_CREATE') // 状态日志
  })

  it('triggers simulation payment logic for PENDING_PAYMENT orders', async () => {
    const mockDetailPending = {
      order: {
        id: 'order_det_001',
        status: 10, // 待支付
        payExpireAt: new Date(Date.now() + 300 * 1000).toISOString(), // 300秒后过期
        goodsOriginAmount: 10000,
        goodsDiscountAmount: 0,
        shippingFee: 0,
        taxFee: 0,
        totalAmount: 10000,
        items: []
      },
      paymentOrder: null,
      stateLogs: [],
      afterSales: [],
      refundOrders: [],
      reservations: []
    }

    vi.mocked(signedFetch).mockImplementation((url) => {
      if (String(url).includes('/api/orders/order_det_001')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(mockDetailPending)
        } as any)
      }
      if (String(url).includes('/api/payments')) {
        return Promise.resolve({
          ok: true
        } as any)
      }
      if (String(url).includes('/api/pay')) {
        // 支付成功后将状态置为已支付，用以测试重新拉取详情
        mockDetailPending.order.status = 20
        return Promise.resolve({
          ok: true
        } as any)
      }
      return Promise.reject(new Error('Unknown url'))
    })

    const wrapper = mount(OrderDetailView)
    await new Promise(resolve => setTimeout(resolve, 50))
    await wrapper.vm.$nextTick()

    // 确认渲染“模拟支付”按钮
    const payBtn = wrapper.findAll('button').find(btn => btn.text().includes('模拟支付'))
    expect(payBtn).toBeDefined()
    expect(wrapper.text()).toContain('待支付')

    // 点击模拟支付
    await payBtn!.trigger('click')
    await wrapper.vm.$nextTick()

    // 验证先后调用了 /api/payments 和 /api/pay 接口
    expect(signedFetch).toHaveBeenCalledWith('/api/payments', expect.objectContaining({ method: 'POST' }))
    expect(signedFetch).toHaveBeenCalledWith('/api/pay', expect.objectContaining({ method: 'POST' }))
  })
})
