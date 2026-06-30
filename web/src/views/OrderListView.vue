<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import Card from '../components/ui/Card.vue'
import Button from '../components/ui/Button.vue'
import Badge from '../components/ui/Badge.vue'

interface OrderInfo {
  id: string
  createdAt: string
  totalAmount: number
  status: number // 1: 待支付, 2: 已支付, 3: 已取消
}

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
const pendingOrders = ref<OrderInfo[]>([])
const allOrders = ref<OrderInfo[]>([]) // Local display orders
const activeOrderId = ref<string | null>(null)
const isPaying = ref(false)
const showSuccessToast = ref(false)

const nowTime = ref(Date.now())
let timer: any = null
let clockTimer: any = null

const fetchMetrics = async () => {
  try {
    const res = await fetch('/api/metrics')
    if (res.ok) {
      const data = await res.json()
      metrics.value = data.metrics
      logs.value = data.logs || []
      
      // Map pending orders from backend
      if (data.pendingOrders) {
        pendingOrders.value = data.pendingOrders
        
        // Sync pending orders into our allOrders list (ensuring no duplicates, keeping local status)
        data.pendingOrders.forEach((po: any) => {
          const found = allOrders.value.find(o => o.id === po.id)
          if (!found) {
            allOrders.value.unshift({
              id: po.id,
              createdAt: po.createdAt,
              totalAmount: po.totalAmount,
              status: po.status
            })
          } else {
            found.status = po.status
          }
        })
      }

      // Check if some orders in our local list were expired/cancelled by the delay worker
      allOrders.value.forEach(o => {
        if (o.status === 1) {
          const stillPending = data.pendingOrders?.some((po: any) => po.id === o.id)
          if (!stillPending) {
            o.status = 3 // Mark as cancelled / expired
          }
        }
      })
    }
  } catch (err) {
    console.error('获取监控指标失败:', err)
  }
}

// Calculate remaining pay time (15 seconds limit from creation)
const getRemainingSeconds = (createdAtStr: string) => {
  const createdTime = new Date(createdAtStr).getTime()
  const limitTime = createdTime + 15 * 1000
  const diff = Math.max(0, Math.ceil((limitTime - nowTime.value) / 1000))
  return diff
}

const handlePay = async (orderId: string) => {
  isPaying.value = true
  try {
    const res = await fetch('/api/pay', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ orderId })
    })
    if (res.ok) {
      showSuccessToast.value = true
      // Update local status immediately
      const order = allOrders.value.find(o => o.id === orderId)
      if (order) order.status = 2
      
      await fetchMetrics()
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
    const res = await fetch('/api/reset', {
      method: 'POST'
    })
    if (res.ok) {
      allOrders.value = []
      pendingOrders.value = []
      await fetchMetrics()
    }
  } catch (err) {
    console.error('系统状态重置失败:', err)
  }
}

onMounted(() => {
  fetchMetrics()
  // Pull data every 1.5s
  timer = setInterval(fetchMetrics, 1500)
  
  // Local clock drives countdown
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
          <p class="hint">在商品详情页选择「立即秒杀抢购」可以生成高并发延迟队列订单</p>
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
              <span class="order-date">{{ new Date(order.createdAt).toLocaleTimeString() }}</span>
            </div>

            <div class="order-body">
              <div class="product-mini-details">
                <span class="product-name">Claude Phone 1 (Haiku 128G)</span>
                <span class="product-qty">数量：1</span>
              </div>
              <div class="price-display">
                实付：<span class="price-val">¥{{ (order.totalAmount / 100).toFixed(2) }}</span>
              </div>
            </div>

            <div class="order-footer">
              <div class="order-status-group">
                <template v-if="order.status === 1">
                  <Badge variant="coral">待支付</Badge>
                  <span class="countdown-hint">
                    支付倒计时: <span class="sec-highlight">{{ getRemainingSeconds(order.createdAt) }}s</span>
                  </span>
                </template>
                <template v-else-if="order.status === 2">
                  <Badge variant="pill" class="status-paid">已支付</Badge>
                </template>
                <template v-else-if="order.status === 3">
                  <Badge variant="pill" class="status-cancelled">已取消 (超时释放)</Badge>
                </template>
              </div>

              <div class="action-group" v-if="order.status === 1">
                <Button
                  @click="handlePay(order.id)"
                  :loading="isPaying"
                  variant="primary"
                  class="pay-btn"
                  :disabled="getRemainingSeconds(order.createdAt) <= 0"
                >
                  去支付
                </Button>
              </div>
            </div>
          </Card>
        </div>
      </div>

      <!-- Right side: Technical Engine Monitor Dashboard -->
      <div class="technical-dashboard-section">
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

.product-mini-details {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.product-name {
  font-family: var(--font-serif);
  font-size: 16px;
  font-weight: 500;
  color: var(--colors-ink);
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
  background-color: rgba(93, 184, 114, 0.1);
  color: var(--colors-success);
}

.status-cancelled {
  background-color: var(--colors-surface-soft);
  color: var(--colors-muted);
}

.pay-btn {
  height: 32px !important;
  font-size: 13px !important;
  padding: 0 16px !important;
}

/* Technical Dashboard Card */
.dashboard-card {
  padding: var(--spacing-lg) !important;
}

.dashboard-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  margin-bottom: var(--spacing-lg);
  border-bottom: 1px solid var(--colors-surface-dark-soft);
  padding-bottom: var(--spacing-sm);
}

.pulse-dot {
  width: 8px;
  height: 8px;
  background-color: var(--colors-success);
  border-radius: 50%;
  animation: pulse 1.5s infinite;
}

@keyframes pulse {
  0% {
    transform: scale(0.95);
    box-shadow: 0 0 0 0 rgba(93, 184, 114, 0.7);
  }
  70% {
    transform: scale(1);
    box-shadow: 0 0 0 6px rgba(93, 184, 114, 0);
  }
  100% {
    transform: scale(0.95);
    box-shadow: 0 0 0 0 rgba(93, 184, 114, 0);
  }
}

.dashboard-title {
  color: var(--colors-on-dark);
  font-family: var(--font-sans);
  font-size: 16px;
  font-weight: 600;
  flex-grow: 1;
}

.reset-btn {
  background: none;
  color: var(--colors-muted-soft);
  font-size: 13px;
}

.reset-btn:hover {
  color: var(--colors-error);
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
}

.stat-item {
  background-color: var(--colors-surface-dark-soft);
  border-radius: var(--rounded-md);
  padding: var(--spacing-md);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xxs);
}

.stat-label {
  font-size: 12px;
  color: var(--colors-on-dark-soft);
}

.stat-value {
  font-family: var(--font-serif);
  font-size: 24px;
  font-weight: 500;
}

.text-coral {
  color: var(--colors-primary);
}

.text-teal {
  color: var(--colors-accent-teal);
}

.log-panel {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.log-header {
  font-size: 12px;
  color: var(--colors-on-dark-soft);
  font-weight: 600;
}

.log-body {
  background-color: var(--colors-surface-dark-soft);
  border-radius: var(--rounded-md);
  padding: var(--spacing-sm);
  font-family: var(--font-mono);
  font-size: 12px;
  min-height: 180px;
  max-height: 240px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.empty-logs {
  color: var(--colors-muted-soft);
  text-align: center;
  margin-top: 60px;
}

.log-item {
  display: flex;
  gap: var(--spacing-xs);
  line-height: 1.4;
}

.log-time {
  color: var(--colors-muted-soft);
}

.log-badge {
  padding: 0 4px;
  border-radius: var(--rounded-xs);
  font-size: 10px;
  font-weight: 600;
}

.log-item.info .log-badge {
  background-color: rgba(93, 184, 166, 0.2);
  color: var(--colors-accent-teal);
}

.log-item.warn .log-badge {
  background-color: rgba(228, 165, 90, 0.2);
  color: var(--colors-accent-amber);
}

.log-item.error .log-badge {
  background-color: rgba(198, 69, 69, 0.2);
  color: var(--colors-error);
}

.log-item.success .log-badge {
  background-color: rgba(93, 184, 114, 0.2);
  color: var(--colors-success);
}

.log-msg {
  color: var(--colors-on-dark-soft);
}

/* Success Toast */
.success-toast {
  position: fixed;
  bottom: var(--spacing-xl);
  right: var(--spacing-xl);
  background-color: var(--colors-surface-dark);
  color: var(--colors-success);
  border: 1px solid var(--colors-success);
  padding: var(--spacing-md);
  border-radius: var(--rounded-md);
  font-size: 14px;
  z-index: 1000;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}
</style>
