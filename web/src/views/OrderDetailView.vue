<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Card from '../components/ui/Card.vue'
import Button from '../components/ui/Button.vue'
import Badge from '../components/ui/Badge.vue'
import { signedFetch } from '../api/request'

const route = useRoute()
const router = useRouter()

const detail = ref<any | null>(null)
const loading = ref(true)
const isPaying = ref(false)
const isPaymentSettling = ref(false)
const nowTime = ref(Date.now())
let clockTimer: any = null

const PAYMENT_SETTLE_POLL_ATTEMPTS = 12
const PAYMENT_SETTLE_POLL_INTERVAL_MS = 1500

const orderId = computed(() => String(route.params.id || ''))
const order = computed(() => detail.value?.order)
const paymentOrder = computed(() => detail.value?.paymentOrder)
const stateLogs = computed(() => detail.value?.stateLogs || [])
const afterSales = computed(() => detail.value?.afterSales || [])
const refundOrders = computed(() => detail.value?.refundOrders || [])
const reservations = computed(() => detail.value?.reservations || [])

const statusText = computed(() => {
  switch (order.value?.status) {
    case 10: return '待支付'
    case 20: return '已支付'
    case 60: return '已取消'
    case 110: return '退款申请中'
    case 120: return '已退款'
    case 130: return '退款已拒绝'
    default: return '未知状态'
  }
})

const remainingSeconds = computed(() => {
  if (!order.value?.payExpireAt) return 0
  return Math.max(0, Math.ceil((new Date(order.value.payExpireAt).getTime() - nowTime.value) / 1000))
})

const payActionLocked = computed(() => isPaying.value || isPaymentSettling.value)

const money = (value?: number) => `¥${(((value || 0) / 100).toFixed(2))}`
const sleep = (ms: number) => new Promise(resolve => window.setTimeout(resolve, ms))

const fetchDetail = async (silent = false) => {
  if (!orderId.value) return false
  if (!silent) loading.value = true
  try {
    const res = await signedFetch(`/api/orders/${orderId.value}`)
    if (res.ok) {
      detail.value = await res.json()
      return true
    }
    return false
  } finally {
    if (!silent) loading.value = false
  }
}

const refreshUntilPaymentSettled = async () => {
  for (let attempt = 0; attempt < PAYMENT_SETTLE_POLL_ATTEMPTS; attempt += 1) {
    await fetchDetail(true)

    if (!order.value || order.value.status !== 10) {
      return
    }

    if (attempt < PAYMENT_SETTLE_POLL_ATTEMPTS - 1) {
      await sleep(PAYMENT_SETTLE_POLL_INTERVAL_MS)
    }
  }
}

const handlePay = async () => {
  if (!order.value || payActionLocked.value) return

  const payingOrderId = order.value.id
  isPaying.value = true
  isPaymentSettling.value = true

  try {
    const paymentRes = await signedFetch('/api/payments', {
      method: 'POST',
      body: JSON.stringify({ orderId: payingOrderId })
    })

    if (paymentRes.ok) {
      await signedFetch('/api/pay', {
        method: 'POST',
        body: JSON.stringify({ orderId: payingOrderId })
      })
    }
  } finally {
    isPaying.value = false
    await refreshUntilPaymentSettled()
    isPaymentSettling.value = false
  }
}

onMounted(() => {
  fetchDetail()
  clockTimer = setInterval(() => {
    nowTime.value = Date.now()
  }, 1000)
})

onUnmounted(() => {
  if (clockTimer) clearInterval(clockTimer)
})
</script>

<template>
  <div class="detail-container">
    <div class="top-row">
      <Button variant="secondary" @click="router.push('/orders')">返回订单</Button>
      <h1 class="typography-display-sm">订单详情</h1>
    </div>

    <div v-if="loading" class="empty-state">正在加载订单详情...</div>
    <div v-else-if="!order" class="empty-state">订单不存在或无权访问</div>

    <div v-else class="detail-grid">
      <section class="main-column">
        <Card variant="cream" class="detail-card">
          <div class="card-title-row">
            <div>
              <div class="muted-label">订单号</div>
              <div class="order-id">{{ order.id }}</div>
            </div>
            <Badge variant="coral" v-if="order.status === 10">{{ statusText }}</Badge>
            <Badge variant="pill" v-else>{{ statusText }}</Badge>
          </div>

          <div class="address-box" v-if="order.receiverName">
            <span>{{ order.receiverName }} {{ order.receiverPhone }}</span>
            <span>{{ order.receiverAddr }}</span>
          </div>

          <div class="items-list">
            <div v-for="item in order.items" :key="item.id" class="line-item">
              <div>
                <div class="item-name">{{ item.sku?.title || `SKU ${item.skuId}` }}</div>
                <div class="muted-label">数量 {{ item.quantity }}</div>
              </div>
              <div class="item-money">
                <span>{{ money(item.originAmount || item.price * item.quantity) }}</span>
                <span v-if="item.itemDiscountAmount > 0" class="discount">-{{ money(item.itemDiscountAmount) }}</span>
              </div>
            </div>
          </div>
        </Card>

        <Card variant="cream" class="detail-card">
          <h3 class="section-title">状态时间线</h3>
          <div v-if="stateLogs.length === 0" class="empty-inline">暂无状态日志</div>
          <div v-else class="timeline">
            <div v-for="log in stateLogs" :key="log.id" class="timeline-row">
              <span class="dot"></span>
              <div>
                <div class="event-name">{{ log.event }}</div>
                <div class="muted-label">{{ log.remark || '状态已更新' }}</div>
                <div class="muted-label">{{ new Date(log.createdAt).toLocaleString() }}</div>
              </div>
            </div>
          </div>
        </Card>
      </section>

      <aside class="side-column">
        <Card variant="cream" class="detail-card">
          <h3 class="section-title">金额快照</h3>
          <div class="summary-row"><span>商品原价</span><span>{{ money(order.goodsOriginAmount) }}</span></div>
          <div class="summary-row"><span>商品优惠</span><span>-{{ money(order.goodsDiscountAmount) }}</span></div>
          <div class="summary-row"><span>运费</span><span>{{ money(order.shippingFee) }}</span></div>
          <div class="summary-row"><span>税费</span><span>{{ money(order.taxFee) }}</span></div>
          <div class="summary-total"><span>应付</span><strong>{{ money(order.totalAmount) }}</strong></div>

          <div v-if="order.status === 10" class="pay-box">
            <span>剩余 {{ remainingSeconds }} 秒</span>
            <Button :loading="payActionLocked" :disabled="remainingSeconds <= 0 || payActionLocked" @click="handlePay" variant="primary">
              {{ isPaymentSettling ? '确认支付结果中...' : '模拟支付' }}
            </Button>
          </div>
        </Card>

        <Card variant="cream" class="detail-card">
          <h3 class="section-title">支付与库存</h3>
          <div class="summary-row"><span>支付单</span><span>{{ paymentOrder?.id || '未创建' }}</span></div>
          <div class="summary-row"><span>支付状态</span><span>{{ paymentOrder?.status || '-' }}</span></div>
          <div class="summary-row"><span>渠道流水</span><span>{{ paymentOrder?.channelTradeNo || '-' }}</span></div>
          <div class="summary-row"><span>库存预占</span><span>{{ reservations.length }} 条</span></div>
        </Card>

        <Card variant="cream" class="detail-card">
          <h3 class="section-title">售后</h3>
          <div class="summary-row"><span>售后单</span><span>{{ afterSales.length }} 条</span></div>
          <div class="summary-row"><span>退款单</span><span>{{ refundOrders.length }} 条</span></div>
        </Card>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.detail-container {
  max-width: 1200px;
  margin: 0 auto;
  padding: var(--spacing-section) var(--spacing-lg);
}

.top-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
}

.detail-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 340px;
  gap: var(--spacing-lg);
}

.main-column,
.side-column {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.detail-card {
  padding: var(--spacing-md) !important;
}

.card-title-row,
.line-item,
.summary-row,
.summary-total,
.pay-box {
  display: flex;
  justify-content: space-between;
  gap: var(--spacing-sm);
  align-items: center;
}

.order-id,
.item-name,
.event-name {
  font-weight: 600;
  color: var(--colors-ink);
}

.muted-label,
.empty-inline {
  font-size: 13px;
  color: var(--colors-muted);
}

.address-box,
.items-list,
.timeline {
  margin-top: var(--spacing-md);
}

.address-box {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: var(--spacing-sm);
  background: var(--colors-surface-soft);
  border-radius: 4px;
}

.line-item {
  padding: var(--spacing-sm) 0;
  border-top: 1px solid var(--colors-hairline-soft);
}

.item-money {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
}

.discount {
  color: var(--colors-coral);
  font-size: 13px;
}

.section-title {
  margin-bottom: var(--spacing-sm);
}

.summary-row,
.summary-total {
  padding: 8px 0;
  border-bottom: 1px solid var(--colors-hairline-soft);
}

.summary-total strong {
  font-size: 20px;
}

.pay-box {
  margin-top: var(--spacing-md);
}

.timeline-row {
  position: relative;
  display: grid;
  grid-template-columns: 16px 1fr;
  gap: var(--spacing-sm);
  padding: 0 0 var(--spacing-md);
}

.dot {
  width: 9px;
  height: 9px;
  margin-top: 5px;
  border-radius: 999px;
  background: var(--colors-coral);
}

.empty-state {
  padding: 72px 20px;
  text-align: center;
  color: var(--colors-muted);
  background: var(--colors-surface-soft);
  border-radius: 8px;
}

@media (max-width: 900px) {
  .detail-grid {
    grid-template-columns: 1fr;
  }
}
</style>