<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import Card from '../components/ui/Card.vue'
import Button from '../components/ui/Button.vue'
import Badge from '../components/ui/Badge.vue'
import { signedFetch } from '../api/request'

interface OrderItem {
  id: number
  skuId: number
  price: number
  quantity: number
  sku?: {
    title: string
    specs: string
  }
}

interface OrderInfo {
  id: string
  createdAt: string
  totalAmount: number
  status: number
  refundReason?: string
  refundProof?: string
  receiverName?: string
  receiverPhone?: string
  receiverAddr?: string
  items?: OrderItem[]
}

const ORDER_STATUS = {
  PENDING_PAYMENT: 10,
  PAID: 20,
  CANCELED: 60,
  REFUND_APPLYING: 110,
  REFUNDED: 120,
  REFUND_REJECTED: 130
} as const

const AFTERSALE_STATUSES = [
  ORDER_STATUS.REFUND_APPLYING,
  ORDER_STATUS.REFUNDED,
  ORDER_STATUS.REFUND_REJECTED
]

interface LogItem {
  time: string
  type: string
  msg: string
}

const metrics = ref({
  seckillStock: 0,
  lockStock: 0,
  ordersPaid: 0,
  revenue: '0.00'
})

const logs = ref<LogItem[]>([])
const pendingOrders = ref<any[]>([])
const allOrders = ref<OrderInfo[]>([]) // Local display orders
const isPaying = ref(false)
const showSuccessToast = ref(false)

const nowTime = ref(Date.now())
let timer: any = null
let clockTimer: any = null

// Refund Dialog State
const showRefundDialog = ref(false)
const refundOrderId = ref('')
const refundReason = ref('商品不想要了/买错了')
const refundProof = ref('https://images.unsplash.com/photo-1557200134-90327ee9fafa?auto=format&fit=crop&w=150&q=80')

// Refund Timelines collapse state
const expandedTimelines = ref<string[]>([])

const fetchMetrics = async () => {
  try {
    const res = await signedFetch('/api/metrics')
    if (res.ok) {
      const data = await res.json()
      metrics.value = data.metrics
      logs.value = data.logs || []
      
      // Update local pending order info
      if (data.pendingOrders) {
        pendingOrders.value = data.pendingOrders
      }
    }
  } catch (err) {
    console.error('获取监控指标失败:', err)
  }
}

const fetchAllOrders = async () => {
  try {
    const res = await signedFetch('/api/orders')
    if (res.ok) {
      const data = await res.json()
      allOrders.value = data || []
    }
  } catch (err) {
    console.error('获取用户订单列表失败:', err)
  }
}

// Calculate remaining pay time (15 seconds limit from creation for seckill, 60s for normal)
const getRemainingSeconds = (order: OrderInfo) => {
  const createdTime = new Date(order.createdAt).getTime()
  // If order id starts with GS, it is a normal order (60s limit), otherwise seckill (15s limit)
  const isNormal = order.id.startsWith('GS-')
  const limitTime = createdTime + (isNormal ? 60 : 15) * 1000
  const diff = Math.max(0, Math.ceil((limitTime - nowTime.value) / 1000))
  return diff
}

const handlePay = async (orderId: string) => {
  isPaying.value = true
  try {
    const res = await signedFetch('/api/pay', {
      method: 'POST',
      body: JSON.stringify({ orderId })
    })
    if (res.ok) {
      showSuccessToast.value = true
      // Update local status
      const order = allOrders.value.find(o => o.id === orderId)
      if (order) order.status = ORDER_STATUS.PAID
      
      await fetchMetrics()
      await fetchAllOrders()
      setTimeout(() => {
        showSuccessToast.value = false
      }, 3000)
    }
  } catch (err) {
    console.error('订单支付失败:', err)
  } finally {
    isPaying.value = false
  }
}

const handleReset = async () => {
  try {
    const res = await signedFetch('/api/reset', {
      method: 'POST'
    })
    if (res.ok) {
      allOrders.value = []
      pendingOrders.value = []
      await fetchMetrics()
      await fetchAllOrders()
    }
  } catch (err) {
    console.error('系统状态重置失败:', err)
  }
}

// Open refund request modal
const openRefund = (orderId: string) => {
  refundOrderId.value = orderId
  refundReason.value = '商品不想要了/买错了'
  showRefundDialog.value = true
}

// Submit refund request
const submitRefund = async () => {
  if (!refundOrderId.value) return
  try {
    const res = await signedFetch(`/api/orders/${refundOrderId.value}/refund`, {
      method: 'POST',
      body: JSON.stringify({
        refundReason: refundReason.value,
        refundProof: refundProof.value
      })
    })
    if (res.ok) {
      showRefundDialog.value = false
      await fetchAllOrders()
      await fetchMetrics()
      alert('退款申请已成功提交，等待商家审核！')
    } else {
      alert('提交退款失败')
    }
  } catch (e) {
    alert('请求错误，提交退款失败')
  }
}

// Simulate Merchant Audit process
const auditRefund = async (orderId: string, action: 'approve' | 'reject') => {
  try {
    const res = await signedFetch(`/api/admin/orders/${orderId}/refund/audit`, {
      method: 'POST',
      body: JSON.stringify({ action })
    })
    if (res.ok) {
      await fetchAllOrders()
      await fetchMetrics()
      alert(action === 'approve' ? '已同意退款，库存与资金已成功原路回退！' : '已拒绝该退款申请。')
    } else {
      alert('审核操作失败')
    }
  } catch (e) {
    alert('请求出错，无法完成审核')
  }
}

// Collapse/expand toggler
const toggleTimeline = (orderId: string) => {
  const index = expandedTimelines.value.indexOf(orderId)
  if (index > -1) {
    expandedTimelines.value.splice(index, 1)
  } else {
    expandedTimelines.value.push(orderId)
  }
}

const isExpanded = (orderId: string) => {
  return expandedTimelines.value.includes(orderId)
}

const isAfterSaleStatus = (status: number) => {
  return AFTERSALE_STATUSES.includes(status as typeof AFTERSALE_STATUSES[number])
}

// Compute refund orders needing merchant audit
const refundingOrders = computed(() => {
  return allOrders.value.filter(o => o.status === ORDER_STATUS.REFUND_APPLYING)
})

onMounted(() => {
  fetchMetrics()
  fetchAllOrders()
  // Pull indicators and metrics every 2.0s
  timer = setInterval(() => {
    fetchMetrics()
    fetchAllOrders()
  }, 2000)
  
  clockTimer = setInterval(() => {
    nowTime.value = Date.now()
  }, 1000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
  if (clockTimer) clearInterval(clockTimer)
})
</script>

<template>
  <div class="orders-container">
    <div class="orders-layout">
      <!-- Left side: Order tracking list -->
      <div class="order-list-section">
        <h2 class="typography-display-sm section-title">订单中心</h2>
        
        <div v-if="allOrders.length === 0" class="empty-state">
          <p>您目前没有任何秒杀或普通订单</p>
          <p class="hint">在商品详情页选择「立即秒杀抢购」或普通商品「去结算」可以创建订单</p>
        </div>

        <div v-else class="orders-flow">
          <Card
            variant="cream"
            v-for="order in allOrders"
            :key="order.id"
            class="order-card"
          >
            <div class="order-header">
              <span class="order-id">单号：{{ order.id }}</span>
              <span class="order-date">{{ new Date(order.createdAt).toLocaleString() }}</span>
            </div>

            <!-- Multi items display -->
            <div class="order-body">
              <div class="order-products-list">
                <div
                  v-for="item in order.items"
                  :key="item.id"
                  class="product-mini-details"
                >
                  <span class="product-name">
                    {{ item.sku?.title || '普通商品' }} 
                    <span class="spec-label" v-if="item.sku?.specs">({{ JSON.parse(item.sku.specs).规格 }})</span>
                  </span>
                  <span class="product-qty">¥{{ (item.price/100).toFixed(2) }} &times; {{ item.quantity }}</span>
                </div>
              </div>
              <div class="price-display">
                实付：<span class="price-val">¥{{ (order.totalAmount / 100).toFixed(2) }}</span>
              </div>
            </div>

            <!-- Address Snapshot display -->
            <div class="address-snapshot-box" v-if="order.receiverName">
              <span class="snapshot-label">配送：</span>
              <span class="snapshot-text">
                {{ order.receiverName }} ({{ order.receiverPhone }}) - {{ order.receiverAddr }}
              </span>
            </div>

            <div class="order-footer">
              <div class="order-status-group">
                <template v-if="order.status === ORDER_STATUS.PENDING_PAYMENT">
                  <Badge variant="coral">待支付</Badge>
                  <span class="countdown-hint">
                    支付倒计时: <span class="sec-highlight">{{ getRemainingSeconds(order) }}s</span>
                  </span>
                </template>
                <template v-else-if="order.status === ORDER_STATUS.PAID">
                  <Badge variant="pill" class="status-paid">已支付</Badge>
                </template>
                <template v-else-if="order.status === ORDER_STATUS.CANCELED">
                  <Badge variant="pill" class="status-cancelled">已取消 (超时自动释放)</Badge>
                </template>
                <template v-else-if="order.status === ORDER_STATUS.REFUND_APPLYING">
                  <Badge variant="coral">退款申请中</Badge>
                </template>
                <template v-else-if="order.status === ORDER_STATUS.REFUNDED">
                  <Badge variant="pill" class="status-cancelled">已全额退款</Badge>
                </template>
                <template v-else-if="order.status === ORDER_STATUS.REFUND_REJECTED">
                  <Badge variant="pill" class="status-paid">退款已拒绝</Badge>
                </template>
              </div>

              <div class="action-group">
                <!-- Pay button -->
                <Button
                  @click="handlePay(order.id)"
                  :loading="isPaying"
                  variant="primary"
                  class="pay-btn"
                  v-if="order.status === ORDER_STATUS.PENDING_PAYMENT"
                  :disabled="getRemainingSeconds(order) <= 0"
                >
                  去支付
                </Button>

                <!-- Refund Button -->
                <Button
                  @click="openRefund(order.id)"
                  variant="secondary"
                  class="refund-btn-action"
                  v-if="order.status === ORDER_STATUS.PAID"
                >
                  申请退款
                </Button>

                <!-- Timeline Toggler -->
                <button
                  class="timeline-toggle-link"
                  @click="toggleTimeline(order.id)"
                  v-if="isAfterSaleStatus(order.status)"
                >
                  {{ isExpanded(order.id) ? '收起进度' : '追踪售后进度' }}
                </button>
              </div>
            </div>

            <!-- Refund Timeline Section -->
            <div class="timeline-box" v-if="isAfterSaleStatus(order.status) && isExpanded(order.id)">
              <div class="timeline-step active">
                <span class="timeline-dot"></span>
                <div class="timeline-content">
                  <div class="step-title">提交退款申请</div>
                  <p class="step-desc">您已提交退款申请。原因: "{{ order.refundReason }}"</p>
                </div>
              </div>
              <div :class="['timeline-step', { active: order.status !== ORDER_STATUS.REFUND_APPLYING }]">
                <span class="timeline-dot"></span>
                <div class="timeline-content">
                  <div class="step-title">商家人工审核</div>
                  <p class="step-desc" v-if="order.status === ORDER_STATUS.REFUND_APPLYING">等待商家安全风控审核中，资金处于托管状态...</p>
                  <p class="step-desc success-msg" v-else-if="order.status === ORDER_STATUS.REFUNDED">商家审核通过。全额资金原路退回，库存自动回滚确认。</p>
                  <p class="step-desc reject-msg" v-else-if="order.status === ORDER_STATUS.REFUND_REJECTED">商家已拒绝退款申请。订单重归交易态。</p>
                </div>
              </div>
            </div>
          </Card>
        </div>
      </div>

      <!-- Right side: Technical Engine Monitor Dashboard -->
      <div class="technical-dashboard-section">
        <!-- Dashboard Card -->
        <Card variant="dark" class="dashboard-card">
          <div class="dashboard-header">
            <span class="pulse-dot"></span>
            <h3 class="dashboard-title">技术引擎看板</h3>
            <button @click="handleReset" class="reset-btn">重置状态</button>
          </div>

          <!-- Stats Grid -->
          <div class="stats-grid">
            <div class="stat-item">
              <span class="stat-label">Valkey 预扣库存</span>
              <span class="stat-value text-coral">{{ metrics.seckillStock }}</span>
            </div>
            <div class="stat-item">
              <span class="stat-label">延迟队列锁定数</span>
              <span class="stat-value text-teal">{{ metrics.lockStock }}</span>
            </div>
            <div class="stat-item">
              <span class="stat-label">已支付订单</span>
              <span class="stat-value">{{ metrics.ordersPaid }}</span>
            </div>
            <div class="stat-item">
              <span class="stat-label">总销售额</span>
              <span class="stat-value">¥{{ metrics.revenue }}</span>
            </div>
          </div>

          <!-- Merchant Refund Review Desk (Audit Panel) -->
          <div class="audit-desk-panel" v-if="refundingOrders.length > 0">
            <div class="panel-header">商家退款审核台</div>
            <div class="audit-items-flow">
              <div
                v-for="ro in refundingOrders"
                :key="ro.id"
                class="audit-item-row"
              >
                <div class="audit-info">
                  <span class="audit-oid">订单: {{ ro.id }}</span>
                  <span class="audit-reason">原因: {{ ro.refundReason }}</span>
                </div>
                <div class="audit-actions">
                  <button class="audit-btn approve" @click="auditRefund(ro.id, 'approve')">批准</button>
                  <button class="audit-btn reject" @click="auditRefund(ro.id, 'reject')">拒绝</button>
                </div>
              </div>
            </div>
          </div>

          <!-- Logger Panel -->
          <div class="log-panel">
            <div class="log-header">
              <span>系统运行日志 (Valkey ZSet & Delay Worker)</span>
            </div>
            <div class="log-body">
              <div v-if="logs.length === 0" class="empty-logs">
                等待高并发活动日志投递...
              </div>
              <div
                v-else
                v-for="(log, idx) in logs"
                :key="idx"
                :class="['log-item', log.type.toLowerCase()]"
              >
                <span class="log-time">[{{ log.time }}]</span>
                <span class="log-badge">{{ log.type }}</span>
                <span class="log-msg">{{ log.msg }}</span>
              </div>
            </div>
          </div>
        </Card>
      </div>
    </div>

    <!-- Refund Reason Application Dialog -->
    <div class="modal-overlay" v-if="showRefundDialog">
      <div class="modal-card">
        <div class="modal-header">
          <h3>申请退款售后</h3>
          <button class="close-btn" @click="showRefundDialog = false">&times;</button>
        </div>
        <div class="modal-body form-modal-body">
          <div class="form-input-unit full-width">
            <label>退款原因</label>
            <select v-model="refundReason" class="refund-select-control">
              <option value="商品不想要了/买错了">商品不想要了/买错了</option>
              <option value="商品有破损/质量问题">商品有破损/质量问题</option>
              <option value="快递迟迟未送达">快递迟迟未送达</option>
              <option value="商家缺货协商退款">商家缺货协商退款</option>
            </select>
          </div>

          <div class="form-input-unit full-width">
            <label>上传退款凭证 (静态演示图链接)</label>
            <Input v-model="refundProof" placeholder="凭证图片地址" />
          </div>

          <div class="form-actions-row">
            <Button @click="showRefundDialog = false" variant="secondary">取消</Button>
            <Button @click="submitRefund" variant="primary">提交退款申请</Button>
          </div>
        </div>
      </div>
    </div>

    <!-- Success Toast -->
    <div v-if="showSuccessToast" class="success-toast">
      支付成功！订单已被延迟队列移除，完成物理归档。
    </div>
  </div>
</template>

<style scoped>
.orders-container {
  max-width: 1200px;
  margin: 0 auto;
  padding: var(--spacing-section) var(--spacing-lg);
  width: 100%;
}

.orders-layout {
  display: grid;
  grid-template-columns: 1.2fr 0.8fr;
  gap: var(--spacing-xl);
  align-items: start;
}

@media (max-width: 992px) {
  .orders-layout {
    grid-template-columns: 1fr;
  }
}

.section-title {
  margin-bottom: var(--spacing-lg);
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 80px 20px;
  color: var(--colors-muted);
  text-align: center;
  background-color: var(--colors-surface-soft);
  border-radius: var(--rounded-lg);
}

.hint {
  font-size: 13px;
  margin-top: 8px;
  color: var(--colors-muted-soft);
}

.orders-flow {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.order-card {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  padding: var(--spacing-md) !important;
}

.order-header {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  color: var(--colors-muted);
  border-bottom: 1px solid var(--colors-hairline-soft);
  padding-bottom: var(--spacing-xs);
}

.order-body {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.order-products-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xxs);
}

.product-mini-details {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.product-name {
  font-family: var(--font-serif);
  font-size: 16px;
  font-weight: 500;
  color: var(--colors-ink);
}

.spec-label {
  font-size: 12px;
  color: var(--colors-muted);
}

.product-qty {
  font-size: 13px;
  color: var(--colors-muted);
}

.price-display {
  font-size: 14px;
  color: var(--colors-body);
}

.price-val {
  font-size: 18px;
  font-weight: 600;
  color: var(--colors-ink);
}

/* Address snapshot line */
.address-snapshot-box {
  background-color: var(--colors-surface-soft);
  padding: var(--spacing-sm);
  border-radius: 4px;
  font-size: 12px;
  display: flex;
  gap: 4px;
  margin-top: -4px;
}

.snapshot-label {
  font-weight: 600;
  color: var(--colors-muted);
}

.snapshot-text {
  color: var(--colors-body);
}

.order-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-top: 1px solid var(--colors-hairline-soft);
  padding-top: var(--spacing-sm);
}

.order-status-group {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.countdown-hint {
  font-size: 13px;
  color: var(--colors-muted);
}

.sec-highlight {
  font-weight: 600;
  color: var(--colors-primary);
}

.status-paid {
  background-color: rgba(40, 167, 69, 0.1);
  color: var(--colors-success);
}

.status-cancelled {
  background-color: var(--colors-surface-soft);
  color: var(--colors-muted);
}

.action-group {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.timeline-toggle-link {
  background: none;
  font-size: 13px;
  color: var(--colors-primary);
  font-weight: 500;
}

.timeline-toggle-link:hover {
  text-decoration: underline;
}

/* Timeline box styling */
.timeline-box {
  border-top: 1px dashed var(--colors-hairline);
  padding-top: var(--spacing-md);
  margin-top: var(--spacing-xs);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.timeline-step {
  display: flex;
  gap: var(--spacing-md);
  position: relative;
}

.timeline-step:not(:last-child)::after {
  content: '';
  position: absolute;
  left: 5px;
  top: 16px;
  bottom: -20px;
  width: 2px;
  background-color: var(--colors-hairline-soft);
}

.timeline-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background-color: var(--colors-hairline);
  margin-top: 4px;
}

.timeline-step.active .timeline-dot {
  background-color: var(--colors-primary);
  box-shadow: 0 0 6px var(--colors-primary);
}

.timeline-content {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.step-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--colors-ink);
}

.step-desc {
  font-size: 12px;
  color: var(--colors-muted);
  margin: 0;
}

.success-msg {
  color: var(--colors-success);
}

.reject-msg {
  color: var(--colors-error);
}

/* Technical Kanban panel */
.dashboard-card {
  padding: var(--spacing-xl) !important;
  color: var(--colors-on-dark);
}

.dashboard-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-lg);
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  padding-bottom: var(--spacing-sm);
}

.pulse-dot {
  width: 8px;
  height: 8px;
  background-color: var(--colors-success);
  border-radius: 50%;
  animation: pulse 1.8s infinite;
}

@keyframes pulse {
  0% {
    box-shadow: 0 0 0 0 rgba(40, 167, 69, 0.7);
  }
  70% {
    box-shadow: 0 0 0 6px rgba(40, 167, 69, 0);
  }
  100% {
    box-shadow: 0 0 0 0 rgba(40, 167, 69, 0);
  }
}

.dashboard-title {
  font-family: var(--font-serif);
  font-size: 18px;
  margin: 0;
  flex-grow: 1;
}

.reset-btn {
  background-color: transparent;
  border: 1px solid var(--colors-primary);
  color: var(--colors-primary);
  padding: 4px 10px;
  border-radius: 4px;
  font-size: 12px;
}

.reset-btn:hover {
  background-color: var(--colors-primary);
  color: var(--colors-on-dark);
}

.stats-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
}

.stat-item {
  background-color: rgba(255, 255, 255, 0.04);
  padding: var(--spacing-md);
  border-radius: var(--rounded-md);
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.stat-label {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.6);
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  font-family: var(--font-serif);
}

.text-coral {
  color: var(--colors-primary);
}

.text-teal {
  color: #00adb5;
}

/* Merchant Review Desk inside Kanban */
.audit-desk-panel {
  background-color: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
  padding: var(--spacing-md);
  border-radius: var(--rounded-md);
  margin-bottom: var(--spacing-lg);
}

.panel-header {
  font-size: 13px;
  font-weight: 600;
  color: var(--colors-primary);
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  padding-bottom: 6px;
  margin-bottom: var(--spacing-sm);
}

.audit-items-flow {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  max-height: 150px;
  overflow-y: auto;
}

.audit-item-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
  background-color: rgba(0, 0, 0, 0.2);
  padding: 8px;
  border-radius: 4px;
}

.audit-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.audit-oid {
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
}

.audit-reason {
  color: rgba(255, 255, 255, 0.6);
}

.audit-actions {
  display: flex;
  gap: 4px;
}

.audit-btn {
  border: none;
  font-size: 10px;
  padding: 4px 8px;
  border-radius: 2px;
  font-weight: 600;
}

.audit-btn.approve {
  background-color: #28a745;
  color: white;
}

.audit-btn.reject {
  background-color: #dc3545;
  color: white;
}

.audit-btn:hover {
  opacity: 0.9;
}

/* Logging console styling */
.log-panel {
  border-top: 1px solid rgba(255, 255, 255, 0.1);
  padding-top: var(--spacing-md);
}

.log-header {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.5);
  margin-bottom: var(--spacing-sm);
}

.log-body {
  background-color: #111;
  border-radius: var(--rounded-md);
  padding: var(--spacing-sm);
  font-family: monospace;
  font-size: 11px;
  height: 200px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.log-item {
  line-height: 1.4;
  word-break: break-all;
}

.log-item.info {
  color: #ccc;
}

.log-item.success {
  color: #28a745;
}

.log-item.warn {
  color: #ffc107;
}

.log-item.error {
  color: #dc3545;
}

.log-time {
  color: #666;
  margin-right: 4px;
}

.log-badge {
  font-weight: 600;
  margin-right: 6px;
}

.success-toast {
  position: fixed;
  bottom: 40px;
  left: 50%;
  transform: translateX(-50%);
  background-color: #28a745;
  color: white;
  padding: var(--spacing-md) var(--spacing-xl);
  border-radius: var(--rounded-md);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  z-index: 1000;
}

/* Modal overlays styles duplicate */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background-color: rgba(24, 23, 21, 0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal-card {
  background-color: var(--colors-canvas);
  border-radius: var(--rounded-lg);
  border: 1px solid var(--colors-hairline);
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.15);
  width: 90%;
  max-width: 480px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.modal-header {
  padding: var(--spacing-md) var(--spacing-xl);
  border-bottom: 1px solid var(--colors-hairline-soft);
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.modal-header h3 {
  margin: 0;
  font-family: var(--font-serif);
  font-size: 18px;
}

.close-btn {
  background: none;
  font-size: 24px;
  color: var(--colors-muted);
  cursor: pointer;
}

.modal-body {
  padding: var(--spacing-xl);
}

.form-modal-body {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.form-input-unit {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.form-input-unit.full-width {
  width: 100%;
}

.form-input-unit label {
  font-size: 12px;
  color: var(--colors-muted);
  font-weight: 500;
}

.refund-select-control {
  padding: 8px 12px;
  border-radius: var(--rounded-md);
  border: 1px solid var(--colors-hairline);
  background-color: var(--colors-surface-soft);
  font-size: 13px;
  color: var(--colors-ink);
  width: 100%;
}

.form-actions-row {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-md);
  margin-top: var(--spacing-md);
}
</style>
