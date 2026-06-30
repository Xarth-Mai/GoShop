<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'

// 菜单和抽屉状态
const isConsoleOpen = ref(true) // 默认开启技术引擎看板，方便演示
const activeOrderId = ref<string | null>(null) // 当前抢购成功的订单ID
const showPaySuccessModal = ref(false) // 支付成功提示
const lastSeckillStatusMsg = ref('') // 秒杀报错消息
const isSeckilling = ref(false) // 抢购按钮的 loading 状态

// 模拟的高并发秒杀数据
const metrics = ref({
  seckillStock: 87, // Valkey 预扣库存剩余
  lockStock: 13,    // 延迟队列中锁定的库存
  ordersPaid: 154,  // 已支付订单数
  revenue: '38,290.00' // 今日销售额
})

// 日志数据结构
interface LogItem {
  time: string
  type: string
  msg: string
}
const logs = ref<LogItem[]>([])

interface OrderInfo {
  id: string
  createdAt: string
  status: number // 1: 待支付, 2: 已支付, 3: 已取消
}
const pendingOrders = ref<OrderInfo[]>([])

// 倒计时刷新用
const nowTime = ref(Date.now())
let countdownTimer: any = null

// 获取后端指标与日志
const fetchMetrics = async () => {
  try {
    const res = await fetch('/api/metrics')
    if (res.ok) {
      const data = await res.json()
      metrics.value = data.metrics
      if (data.logs) {
        logs.value = data.logs
      }
      if (data.pendingOrders) {
        pendingOrders.value = data.pendingOrders
        
        // 如果 activeOrderId 在 pendingOrders 里找不到了，且当前支付弹窗还开着
        // 且它不是被支付成功的，说明它已经过期被后端回收了
        if (activeOrderId.value) {
          const isStillPending = data.pendingOrders.some((o: OrderInfo) => o.id === activeOrderId.value)
          if (!isStillPending && !showPaySuccessModal.value) {
            // 自动清除当前订单，提示超时
            activeOrderId.value = null
            lastSeckillStatusMsg.value = '订单超时未支付，库存已由后台自动释放！'
            setTimeout(() => {
              lastSeckillStatusMsg.value = ''
            }, 5000)
          }
        }
      } else {
        pendingOrders.value = []
        if (activeOrderId.value && !showPaySuccessModal.value) {
          activeOrderId.value = null
        }
      }
    }
  } catch (err) {
    console.error('获取指标失败:', err)
  }
}

// 计算订单剩余支付秒数 (后端延迟队列设为 15 秒)
const getRemainingSeconds = (createdAtStr: string) => {
  const createdTime = new Date(createdAtStr).getTime()
  const limitTime = createdTime + 15 * 1000
  const diff = Math.max(0, Math.ceil((limitTime - nowTime.value) / 1000))
  return diff
}

// 模拟秒杀下单
const handleSeckill = async () => {
  if (metrics.value.seckillStock <= 0) return
  isSeckilling.value = true
  lastSeckillStatusMsg.value = ''
  
  try {
    const res = await fetch('/api/seckill', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      }
    })
    const data = await res.json()
    if (res.ok && data.status === 'success') {
      activeOrderId.value = data.orderId
      showPaySuccessModal.value = false
      // 成功后立即拉取一次最新状态
      await fetchMetrics()
    } else {
      lastSeckillStatusMsg.value = data.message || '秒杀失败，请重试'
      setTimeout(() => {
        lastSeckillStatusMsg.value = ''
      }, 4000)
    }
  } catch (err) {
    console.error('秒杀接口请求失败:', err)
    lastSeckillStatusMsg.value = '网络请求失败，请检查服务状态'
  } finally {
    isSeckilling.value = false
  }
}

// 模拟支付
const handlePay = async (orderId: string) => {
  try {
    const res = await fetch('/api/pay', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ orderId })
    })
    if (res.ok) {
      showPaySuccessModal.value = true
      activeOrderId.value = null
      await fetchMetrics()
      setTimeout(() => {
        showPaySuccessModal.value = false
      }, 4000)
    }
  } catch (err) {
    console.error('支付失败:', err)
  }
}

// 重置库存和系统状态
const handleReset = async () => {
  try {
    const res = await fetch('/api/reset', {
      method: 'POST'
    })
    if (res.ok) {
      activeOrderId.value = null
      showPaySuccessModal.value = false
      lastSeckillStatusMsg.value = ''
      await fetchMetrics()
    }
  } catch (err) {
    console.error('重置库存失败:', err)
  }
}

// 动态总库存进度条（初始总量设定为 100，或以当前总数动态适应）
const stockPercentage = computed(() => {
  const total = 100
  const current = metrics.value.seckillStock
  return Math.min(100, Math.max(0, (current / total) * 100))
})

let metricsTimer: any = null
onMounted(() => {
  fetchMetrics()
  // 1.5秒刷新一次后端数据
  metricsTimer = setInterval(fetchMetrics, 1500)
  
  // 每秒刷新一次本地当前时间，驱动倒计时
  countdownTimer = setInterval(() => {
    nowTime.value = Date.now()
  }, 1000)
})

onUnmounted(() => {
  if (metricsTimer) clearInterval(metricsTimer)
  if (countdownTimer) clearInterval(countdownTimer)
})
</script>

<template>
  <div class="goshop-mall" :class="{ 'console-layout': isConsoleOpen }">
    <!-- Top Navigation Header -->
    <header class="mall-header">
      <div class="nav-brand">
        <svg class="spike-mark animate-pulse" width="22" height="22" viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 2L14.8 9.2L22 12L14.8 14.8L12 22L9.2 14.8L2 12L9.2 9.2Z" />
        </svg>
        <span class="brand-title">GoShop <span class="brand-badge">秒杀专区</span></span>
      </div>
      
      <div class="nav-center-tips">
        <span class="tech-badge">
          <span class="pulse-dot"></span>
          Valkey 内存原子锁杜绝超卖
        </span>
      </div>

      <div class="nav-actions">
        <button class="console-toggle-btn" :class="{ active: isConsoleOpen }" @click="isConsoleOpen = !isConsoleOpen">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="2" y="3" width="20" height="14" rx="2" ry="2"/>
            <line x1="8" y1="21" x2="16" y2="21"/>
            <line x1="12" y1="17" x2="12" y2="21"/>
          </svg>
          {{ isConsoleOpen ? '隐藏技术看板' : '显示技术看板' }}
        </button>
      </div>
    </header>

    <div class="mall-body">
      <!-- Left: Buyer Storefront (E-commerce Product View) -->
      <main class="product-view">
        <div class="product-container">
          <!-- Product Image Showcase -->
          <div class="product-gallery">
            <div class="main-image-card">
              <img :src="'/phone.png'" alt="GoShop Meta Phone Ultra" class="product-image" />
              <div class="image-overlay-glow"></div>
            </div>
            <div class="product-highlights">
              <div class="highlight-item">
                <span class="icon">⚡</span>
                <div class="text">
                  <h4>Valkey 原子扣减</h4>
                  <p>Lua脚本保障内存级原子性</p>
                </div>
              </div>
              <div class="highlight-item">
                <span class="icon">⏳</span>
                <div class="text">
                  <h4>15s 延迟队列</h4>
                  <p>超时订单自动回收库存</p>
                </div>
              </div>
              <div class="highlight-item">
                <span class="icon">💾</span>
                <div class="text">
                  <h4>PG 读写分离</h4>
                  <p>高并发最终一致性落库</p>
                </div>
              </div>
            </div>
          </div>

          <!-- Product Details & Purchase Area -->
          <div class="product-info-panel">
            <div class="badge-row">
              <span class="status-badge live">限时秒杀中</span>
              <span class="status-badge free-shipping">免邮费</span>
            </div>
            
            <h1 class="product-title">GoShop Meta Phone Ultra</h1>
            <p class="product-edition">极客联名限量版 (Cyber Edition)</p>
            
            <p class="product-description">
              专为高并发压力测试打造的旗舰级智能终端。内置秒杀专用极速预热缓存，支持十万级 QPS 原子扣减，搭配基于 Redis ZSet 延迟队列的 15 秒未付款订单自动回退算法，确保在高并发极端场景下全链路数据最终一致、不超卖、不漏单。
            </p>

            <div class="product-meta-specs">
              <div class="spec-tag">高并发优化</div>
              <div class="spec-tag">Lua 预扣库存</div>
              <div class="spec-tag">延迟队列防锁死</div>
            </div>

            <!-- Price Zone -->
            <div class="price-card">
              <div class="price-details">
                <span class="price-label">秒杀惊喜价</span>
                <div class="price-row">
                  <span class="currency">¥</span>
                  <span class="amount">399</span>
                  <span class="decimal">.00</span>
                  <span class="original-price">¥8,999.00</span>
                </div>
              </div>
              <div class="countdown-clock">
                <span class="clock-label">距秒杀结束还剩</span>
                <div class="clock-values">
                  <span class="num">02</span><span class="sep">:</span>
                  <span class="num">15</span><span class="sep">:</span>
                  <span class="num">30</span>
                </div>
              </div>
            </div>

            <!-- Stock progress bar -->
            <div class="stock-progress-zone">
              <div class="stock-text">
                <span class="stock-remain">仅剩 <strong>{{ metrics.seckillStock }}</strong> 件</span>
                <span class="stock-total">总库存 100 件</span>
              </div>
              <div class="progress-bar-container">
                <div 
                  class="progress-bar" 
                  :style="{ width: stockPercentage + '%' }"
                  :class="{ 'low-stock': metrics.seckillStock <= 15 }"
                ></div>
              </div>
            </div>

            <!-- Action buttons -->
            <div class="purchase-actions">
              <button 
                class="seckill-btn" 
                :class="{ 
                  'loading': isSeckilling, 
                  'sold-out': metrics.seckillStock <= 0 
                }" 
                :disabled="metrics.seckillStock <= 0 || isSeckilling"
                @click="handleSeckill"
              >
                <span v-if="isSeckilling" class="spinner"></span>
                <span v-else-if="metrics.seckillStock <= 0">已抢光 (Out of Stock)</span>
                <span v-else>立即秒杀抢购</span>
              </button>
              
              <div class="user-tips">
                * 抢购成功后请在 15 秒内模拟付款，超时订单将被系统强制取消。
              </div>
            </div>

            <!-- Client Status Messages -->
            <transition name="fade">
              <div v-if="lastSeckillStatusMsg" class="client-alert-banner" :class="{ 'warning': lastSeckillStatusMsg.includes('超时') }">
                <span class="alert-icon">⚠️</span>
                <span class="alert-text">{{ lastSeckillStatusMsg }}</span>
              </div>
            </transition>
          </div>
        </div>

        <!-- Float Payment Modal -->
        <transition name="modal-fade">
          <div class="payment-modal-backdrop" v-if="activeOrderId || showPaySuccessModal">
            <!-- Modal Card -->
            <div class="payment-modal-card glassmorphism">
              <!-- Success Screen -->
              <div v-if="showPaySuccessModal" class="payment-success-screen">
                <div class="success-icon-anim animate-bounce">
                  <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
                    <polyline points="20 6 9 17 4 12" />
                  </svg>
                </div>
                <h3 class="success-title">🎉 模拟支付成功！</h3>
                <p class="success-desc">订单已写入 PostgreSQL 数据库，状态更新为【已支付】。</p>
                <div class="payment-receipt">
                  <div class="receipt-row"><span>交易单号:</span><span class="font-mono">Order Success</span></div>
                  <div class="receipt-row"><span>支付金额:</span><span class="price-val">¥399.00</span></div>
                </div>
              </div>

              <!-- Paying Screen -->
              <div v-else-if="activeOrderId" class="payment-pay-screen">
                <div class="modal-header">
                  <span class="modal-badge animate-pulse">待支付锁定中</span>
                  <h3 class="modal-title">恭喜！抢购已锁定</h3>
                </div>
                
                <div class="order-detail-box">
                  <div class="detail-row">
                    <span class="label">订单 ID</span>
                    <span class="value font-mono">{{ activeOrderId }}</span>
                  </div>
                  <div class="detail-row">
                    <span class="label">抢购商品</span>
                    <span class="value">GoShop Meta Phone Ultra x1</span>
                  </div>
                  <div class="detail-row">
                    <span class="label">支付金额</span>
                    <span class="value price-highlight">¥399.00</span>
                  </div>
                </div>

                <!-- 15s Timer Progress Circle/Bar -->
                <div class="timeout-alert-box">
                  <div class="timer-warning">
                    <span class="warning-icon">⏳</span>
                    <div class="warning-text">
                      <span class="title">请在 15 秒内付款</span>
                      <p>Valkey 延迟队列已启动，超时将自动回退库存并作废订单。</p>
                    </div>
                  </div>
                  <div class="countdown-bar-wrapper">
                    <div class="countdown-text">
                      剩余支付时间：<strong>{{ getRemainingSeconds(pendingOrders.find(o => o.id === activeOrderId)?.createdAt || new Date().toISOString()) }}</strong> 秒
                    </div>
                    <div class="countdown-line">
                      <div 
                        class="countdown-fill" 
                        :style="{ 
                          width: (getRemainingSeconds(pendingOrders.find(o => o.id === activeOrderId)?.createdAt || new Date().toISOString()) / 15) * 100 + '%' 
                        }"
                      ></div>
                    </div>
                  </div>
                </div>

                <div class="modal-actions">
                  <button class="btn-primary pay-now-btn" @click="handlePay(activeOrderId)">
                    立即支付 (¥399.00)
                  </button>
                  <button class="btn-secondary cancel-pay-btn" @click="activeOrderId = null">
                    暂不支付
                  </button>
                </div>
              </div>
            </div>
          </div>
        </transition>

        <!-- Multi-order Pending List (For multiple orders in the delay queue) -->
        <div class="multi-orders-section" v-if="pendingOrders.length > 0">
          <div class="orders-card-header">
            <span class="badge">排队中</span>
            <h3>当前处于延迟锁定中的秒杀订单 ({{ pendingOrders.length }})</h3>
          </div>
          <div class="orders-list">
            <div class="order-list-row" v-for="order in pendingOrders" :key="order.id" :class="{ 'highlight': order.id === activeOrderId }">
              <div class="order-info-meta">
                <span class="order-id">单号：<code class="font-mono">{{ order.id }}</code></span>
                <span class="timer-rem">⏳ 剩 <strong>{{ getRemainingSeconds(order.createdAt) }}</strong> 秒释放</span>
              </div>
              <div class="order-action-btn">
                <span class="price">¥399.00</span>
                <button class="small-pay-btn" @click="handlePay(order.id)">
                  支付
                </button>
              </div>
            </div>
          </div>
        </div>
      </main>

      <!-- Right: Technical Engine Glassbox Console -->
      <transition name="console-slide">
        <aside class="tech-console" v-show="isConsoleOpen">
          <div class="console-header">
            <div class="console-title-row">
              <span class="console-dot-indicator"></span>
              <h3>GoShop Technical Console</h3>
            </div>
            <button class="console-reset-btn" @click="handleReset" title="重置库存和清空测试数据">
              重置系统状态
            </button>
          </div>

          <div class="console-scroll-content">
            <!-- 4 Grid Core Metrics -->
            <div class="console-metrics-grid">
              <div class="metric-card">
                <div class="title">内存级原子库存</div>
                <div class="value valkey">{{ metrics.seckillStock }}</div>
                <div class="unit">SKU 缓存件数</div>
              </div>
              <div class="metric-card">
                <div class="title">延迟队列锁库</div>
                <div class="value delay">{{ metrics.lockStock }}</div>
                <div class="unit">ZSet 排队笔数</div>
              </div>
              <div class="metric-card">
                <div class="title">已落库订单数</div>
                <div class="value postgres">{{ metrics.ordersPaid }}</div>
                <div class="unit">PG 最终已付数</div>
              </div>
              <div class="metric-card">
                <div class="title">今日累计销售额</div>
                <div class="value revenue">¥{{ metrics.revenue }}</div>
                <div class="unit">读写分离实时统计</div>
              </div>
            </div>

            <!-- Valkey / Redis CLI Journal Logs -->
            <div class="cli-journal-card">
              <div class="cli-header">
                <div class="cli-dots">
                  <span class="cli-dot r"></span>
                  <span class="cli-dot y"></span>
                  <span class="cli-dot g"></span>
                </div>
                <span class="cli-title">Valkey / Redis CLI Journal</span>
              </div>
              <div class="cli-terminal">
                <div class="log-row" v-for="(log, i) in logs" :key="i">
                  <span class="log-time">[{{ log.time }}]</span>
                  <span class="log-type" :class="log.type.toLowerCase()">{{ log.type }}</span>
                  <span class="log-msg">{{ log.msg }}</span>
                </div>
                <div v-if="metrics.seckillStock === 0" class="log-row out-of-stock-alert">
                  <span class="log-type error">ERR_OUT</span>
                  <span class="log-msg">Valkey stock is 0. Lua transaction aborted.</span>
                </div>
                <div v-if="logs.length === 0" class="log-row empty-log">
                  <span class="log-msg">> Console listening to metrics stream...</span>
                </div>
              </div>
            </div>

            <!-- Tech Stack Infographics -->
            <div class="tech-stack-card">
              <h4>分布式高并发架构原理</h4>
              <ul class="principles-list">
                <li>
                  <span class="num">1</span>
                  <p><strong>Lua脚本原子扣减</strong>：请求到达后在 Valkey (Redis) 缓存层执行 Lua 脚本，进行原子扣减并防超卖，速度极快。</p>
                </li>
                <li>
                  <span class="num">2</span>
                  <p><strong>ZSet延迟订单回收</strong>：秒杀成功即写入数据库【待支付】状态，并推入延迟队列。若15秒未支付，Worker协程将订单标记为【已取消】并执行 Redis 缓存库存回滚。</p>
                </li>
                <li>
                  <span class="num">3</span>
                  <p><strong>PostgreSQL落库与读写分离</strong>：仅在支付成功或确认下单时落库，极大减轻持久化数据库写入压力，读取使用只读副本提升查询效率。</p>
                </li>
              </ul>
            </div>
          </div>
        </aside>
      </transition>
    </div>
  </div>
</template>

<style scoped>
/* 本地组件内样式，与外部 style.css 形成互补 */
.goshop-mall {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background-color: var(--colors-canvas);
  color: var(--colors-ink);
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  overflow-x: hidden;
}

/* Header */
.mall-header {
  height: 64px;
  background-color: rgba(250, 249, 245, 0.85);
  backdrop-filter: blur(12px);
  border-bottom: 1px solid var(--colors-hairline);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 var(--spacing-lg);
  position: sticky;
  top: 0;
  z-index: 100;
}

.nav-brand {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.spike-mark {
  color: var(--colors-primary);
}

.brand-title {
  font-family: var(--font-serif);
  font-size: 22px;
  font-weight: 700;
  letter-spacing: -0.5px;
}

.brand-badge {
  font-family: var(--font-sans);
  font-size: 11px;
  font-weight: 600;
  background: linear-gradient(135deg, var(--colors-primary), #d97706);
  color: var(--colors-on-primary);
  padding: 2px 8px;
  border-radius: var(--rounded-pill);
  margin-left: 6px;
  letter-spacing: 0.5px;
}

.nav-center-tips {
  display: flex;
  align-items: center;
}

.tech-badge {
  font-size: 12px;
  font-weight: 500;
  color: var(--colors-muted);
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background-color: var(--colors-surface-soft);
  padding: 6px 14px;
  border-radius: var(--rounded-pill);
  border: 1px solid var(--colors-hairline-soft);
}

.pulse-dot {
  width: 8px;
  height: 8px;
  background-color: var(--colors-success);
  border-radius: var(--rounded-full);
  box-shadow: 0 0 8px var(--colors-success);
  animation: pulse 2s infinite;
}

.console-toggle-btn {
  background-color: var(--colors-surface-card);
  border: 1px solid var(--colors-hairline);
  color: var(--colors-ink);
  font-size: 13px;
  font-weight: 500;
  padding: 6px 14px;
  border-radius: var(--rounded-md);
  display: inline-flex;
  align-items: center;
  gap: 6px;
  transition: all 0.2s ease;
}

.console-toggle-btn:hover {
  background-color: var(--colors-surface-cream-strong);
}

.console-toggle-btn.active {
  background-color: var(--colors-surface-dark);
  color: var(--colors-on-dark);
  border-color: var(--colors-surface-dark);
}

/* Mall Body Layout */
.mall-body {
  flex: 1;
  display: flex;
  position: relative;
  overflow: hidden;
}

/* Layout logic when technical console is active */
.console-layout .product-view {
  margin-right: 420px; /* Leave space for sliding panel */
}

/* Product View (Buyer Area) */
.product-view {
  flex: 1;
  padding: var(--spacing-xl);
  max-width: 1100px;
  margin: 0 auto;
  width: 100%;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.product-container {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--spacing-xl);
  background-color: var(--colors-canvas);
  margin-top: 10px;
}

/* Product Gallery */
.product-gallery {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

.main-image-card {
  position: relative;
  background-color: #0c0b0a; /* Dark background matches phone glow */
  border-radius: var(--rounded-xl);
  overflow: hidden;
  aspect-ratio: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(232, 165, 90, 0.15);
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.15);
}

.product-image {
  max-width: 100%;
  max-height: 100%;
  object-fit: cover;
  transition: transform 0.5s ease;
}

.main-image-card:hover .product-image {
  transform: scale(1.03);
}

.image-overlay-glow {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 40%;
  background: linear-gradient(to top, rgba(232, 165, 90, 0.08), transparent);
  pointer-events: none;
}

.product-highlights {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--spacing-sm);
}

.highlight-item {
  background-color: var(--colors-surface-card);
  padding: var(--spacing-md) 10px;
  border-radius: var(--rounded-lg);
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  gap: 6px;
  border: 1px solid var(--colors-hairline-soft);
}

.highlight-item .icon {
  font-size: 22px;
}

.highlight-item h4 {
  font-family: var(--font-sans);
  font-size: 12px;
  font-weight: 600;
  color: var(--colors-ink);
  margin-bottom: 2px;
}

.highlight-item p {
  font-size: 10px;
  color: var(--colors-muted);
  line-height: 1.3;
}

/* Product Info Panel */
.product-info-panel {
  display: flex;
  flex-direction: column;
}

.badge-row {
  display: flex;
  gap: 8px;
  margin-bottom: var(--spacing-sm);
}

.status-badge {
  font-size: 11px;
  font-weight: 600;
  padding: 4px 10px;
  border-radius: var(--rounded-sm);
}

.status-badge.live {
  background-color: rgba(204, 120, 92, 0.1);
  color: var(--colors-primary);
  border: 1px solid rgba(204, 120, 92, 0.2);
}

.status-badge.free-shipping {
  background-color: var(--colors-surface-soft);
  color: var(--colors-muted);
  border: 1px solid var(--colors-hairline-soft);
}

.product-title {
  font-size: 32px;
  font-weight: 700;
  line-height: 1.15;
  margin-bottom: 4px;
}

.product-edition {
  font-size: 16px;
  color: var(--colors-primary);
  font-weight: 600;
  letter-spacing: 0.5px;
  margin-bottom: var(--spacing-md);
  font-family: var(--font-sans);
}

.product-description {
  font-size: 14px;
  line-height: 1.6;
  color: var(--colors-body);
  margin-bottom: var(--spacing-md);
}

.product-meta-specs {
  display: flex;
  gap: 8px;
  margin-bottom: var(--spacing-lg);
}

.spec-tag {
  font-size: 11px;
  background-color: var(--colors-surface-soft);
  color: var(--colors-muted);
  padding: 3px 10px;
  border-radius: var(--rounded-xs);
  border: 1px solid var(--colors-hairline);
}

/* Price Card */
.price-card {
  background: linear-gradient(135deg, var(--colors-surface-dark), var(--colors-surface-dark-elevated));
  color: var(--colors-on-dark);
  border-radius: var(--rounded-lg);
  padding: var(--spacing-md) var(--spacing-lg);
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-lg);
  box-shadow: 0 4px 15px rgba(0, 0, 0, 0.1);
  border: 1px solid var(--colors-surface-dark-soft);
}

.price-label {
  font-size: 12px;
  color: var(--colors-on-dark-soft);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  display: block;
  margin-bottom: 2px;
}

.price-row {
  display: flex;
  align-items: baseline;
}

.price-row .currency {
  font-size: 18px;
  font-weight: 600;
  color: var(--colors-accent-amber);
}

.price-row .amount {
  font-family: var(--font-serif);
  font-size: 38px;
  font-weight: 700;
  color: var(--colors-accent-amber);
  line-height: 1;
}

.price-row .decimal {
  font-size: 18px;
  font-weight: 600;
  color: var(--colors-accent-amber);
  margin-right: var(--spacing-md);
}

.price-row .original-price {
  font-size: 13px;
  text-decoration: line-through;
  color: var(--colors-on-dark-soft);
}

.countdown-clock {
  text-align: right;
}

.clock-label {
  font-size: 11px;
  color: var(--colors-on-dark-soft);
  display: block;
  margin-bottom: 6px;
}

.clock-values {
  display: flex;
  align-items: center;
  gap: 3px;
}

.clock-values .num {
  background-color: rgba(255, 255, 255, 0.1);
  padding: 4px 8px;
  border-radius: var(--rounded-xs);
  font-family: var(--font-mono);
  font-size: 14px;
  font-weight: 600;
  color: var(--colors-on-dark);
}

.clock-values .sep {
  font-size: 12px;
  color: var(--colors-on-dark-soft);
  font-weight: bold;
}

/* Stock Progress Zone */
.stock-progress-zone {
  margin-bottom: var(--spacing-lg);
}

.stock-text {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  margin-bottom: 6px;
}

.stock-remain strong {
  font-size: 16px;
  color: var(--colors-primary);
}

.stock-total {
  color: var(--colors-muted-soft);
}

.progress-bar-container {
  height: 8px;
  background-color: var(--colors-surface-soft);
  border-radius: var(--rounded-pill);
  overflow: hidden;
  border: 1px solid var(--colors-hairline-soft);
}

.progress-bar {
  height: 100%;
  background: linear-gradient(to right, #e8a55a, var(--colors-primary));
  border-radius: var(--rounded-pill);
  transition: width 0.8s cubic-bezier(0.4, 0, 0.2, 1);
}

.progress-bar.low-stock {
  background: linear-gradient(to right, var(--colors-primary), var(--colors-error));
  animation: pulse-border 1.5s infinite;
}

/* Purchase Actions */
.purchase-actions {
  margin-bottom: var(--spacing-lg);
}

.seckill-btn {
  width: 100%;
  height: 52px;
  background: linear-gradient(135deg, var(--colors-primary), #bf5330);
  color: var(--colors-on-primary);
  font-size: 18px;
  font-weight: 600;
  border-radius: var(--rounded-lg);
  box-shadow: 0 4px 15px rgba(204, 120, 92, 0.25);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  transition: all 0.2s ease;
  letter-spacing: 1px;
}

.seckill-btn:hover:not(:disabled) {
  background: linear-gradient(135deg, var(--colors-primary-active), #993b1d);
  transform: translateY(-2px);
  box-shadow: 0 6px 20px rgba(204, 120, 92, 0.35);
}

.seckill-btn:active:not(:disabled) {
  transform: translateY(0);
}

.seckill-btn:disabled {
  background: var(--colors-primary-disabled);
  color: var(--colors-muted-soft);
  box-shadow: none;
  cursor: not-allowed;
}

.seckill-btn.sold-out {
  background: #a8a29e !important;
  color: #f5f5f4 !important;
}

.user-tips {
  font-size: 12px;
  color: var(--colors-muted);
  text-align: center;
  margin-top: var(--spacing-xs);
}

.client-alert-banner {
  background-color: rgba(204, 120, 92, 0.08);
  border: 1px solid rgba(204, 120, 92, 0.2);
  color: var(--colors-primary);
  padding: 10px 14px;
  border-radius: var(--rounded-md);
  margin-top: 10px;
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.client-alert-banner.warning {
  background-color: rgba(232, 165, 90, 0.08);
  border-color: rgba(232, 165, 90, 0.2);
  color: #bf721d;
}

/* Float Payment Modal Backdrop */
.payment-modal-backdrop {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: rgba(24, 23, 21, 0.45);
  backdrop-filter: blur(8px);
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-lg);
}

.payment-modal-card {
  width: 100%;
  max-width: 440px;
  background-color: rgba(255, 255, 255, 0.85);
  border: 1px solid rgba(232, 165, 90, 0.25);
  border-radius: var(--rounded-xl);
  padding: var(--spacing-xl);
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.15);
  position: relative;
}

.payment-modal-card.glassmorphism {
  background: rgba(250, 249, 245, 0.85);
  backdrop-filter: blur(16px);
}

/* Modal Pay Screen */
.payment-pay-screen .modal-header {
  margin-bottom: var(--spacing-md);
  text-align: center;
}

.modal-badge {
  display: inline-block;
  font-size: 11px;
  font-weight: 600;
  color: var(--colors-accent-amber);
  background-color: rgba(232, 165, 90, 0.12);
  border: 1px solid rgba(232, 165, 90, 0.2);
  padding: 3px 10px;
  border-radius: var(--rounded-pill);
  margin-bottom: 6px;
  text-transform: uppercase;
}

.modal-title {
  font-size: 20px;
  font-weight: 700;
  color: var(--colors-ink);
}

.order-detail-box {
  background-color: rgba(239, 233, 222, 0.4);
  border-radius: var(--rounded-lg);
  padding: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
  border: 1px solid var(--colors-hairline-soft);
}

.detail-row {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  padding: var(--spacing-xs) 0;
  border-bottom: 1px solid rgba(230, 223, 216, 0.5);
}

.detail-row:last-child {
  border-bottom: none;
}

.detail-row .label {
  color: var(--colors-muted);
}

.detail-row .value {
  font-weight: 600;
  color: var(--colors-ink);
}

.price-highlight {
  color: var(--colors-primary) !important;
  font-size: 16px;
}

/* Timeout Alert Box */
.timeout-alert-box {
  background-color: rgba(232, 165, 90, 0.08);
  border: 1px dashed rgba(232, 165, 90, 0.3);
  border-radius: var(--rounded-lg);
  padding: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
}

.timer-warning {
  display: flex;
  gap: 10px;
  margin-bottom: var(--spacing-xs);
}

.warning-icon {
  font-size: 18px;
}

.warning-text .title {
  font-size: 13px;
  font-weight: 600;
  color: #bf721d;
  display: block;
}

.warning-text p {
  font-size: 11px;
  color: var(--colors-muted);
  line-height: 1.3;
}

.countdown-bar-wrapper {
  margin-top: 10px;
}

.countdown-text {
  font-size: 12px;
  color: var(--colors-body);
  margin-bottom: 4px;
}

.countdown-line {
  height: 4px;
  background-color: rgba(232, 165, 90, 0.15);
  border-radius: var(--rounded-pill);
  overflow: hidden;
}

.countdown-fill {
  height: 100%;
  background-color: var(--colors-accent-amber);
  border-radius: var(--rounded-pill);
  transition: width 1s linear;
}

.modal-actions {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.pay-now-btn {
  width: 100%;
  height: 44px;
  background-color: var(--colors-accent-amber);
  color: #fff;
  border-radius: var(--rounded-md);
  font-weight: 600;
  box-shadow: 0 4px 12px rgba(232, 165, 90, 0.2);
}

.pay-now-btn:hover {
  background-color: #d97706;
}

.cancel-pay-btn {
  width: 100%;
  height: 40px;
}

/* Success Screen */
.payment-success-screen {
  text-align: center;
  padding: var(--spacing-md) 0;
}

.success-icon-anim {
  width: 64px;
  height: 64px;
  background-color: rgba(93, 184, 114, 0.15);
  color: var(--colors-success);
  border-radius: var(--rounded-full);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-bottom: var(--spacing-md);
}

.success-title {
  font-size: 20px;
  font-weight: 700;
  color: var(--colors-success);
  margin-bottom: 6px;
}

.success-desc {
  font-size: 13px;
  color: var(--colors-muted);
  margin-bottom: var(--spacing-lg);
}

.payment-receipt {
  background-color: rgba(93, 184, 114, 0.05);
  border: 1px solid rgba(93, 184, 114, 0.15);
  border-radius: var(--rounded-lg);
  padding: var(--spacing-md);
  text-align: left;
}

.receipt-row {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  padding: 6px 0;
}

.receipt-row span:first-child {
  color: var(--colors-muted);
}

.receipt-row .price-val {
  font-weight: 700;
  color: var(--colors-success);
}

/* Multi-order pending list below */
.multi-orders-section {
  margin-top: var(--spacing-xl);
  background-color: var(--colors-surface-soft);
  border: 1px solid var(--colors-hairline);
  border-radius: var(--rounded-lg);
  padding: var(--spacing-lg);
}

.orders-card-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-md);
  border-bottom: 1px solid var(--colors-hairline-soft);
  padding-bottom: var(--spacing-xs);
}

.orders-card-header h3 {
  font-family: var(--font-sans);
  font-size: 15px;
  font-weight: 600;
  color: var(--colors-ink);
}

.orders-card-header .badge {
  font-size: 10px;
  font-weight: 600;
  color: #bf721d;
  background-color: rgba(232, 165, 90, 0.12);
  padding: 2px 8px;
  border-radius: var(--rounded-pill);
}

.orders-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.order-list-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background-color: var(--colors-canvas);
  border: 1px solid var(--colors-hairline-soft);
  padding: var(--spacing-sm);
  border-radius: var(--rounded-md);
  transition: all 0.2s ease;
}

.order-list-row.highlight {
  border-color: var(--colors-accent-amber);
  box-shadow: 0 0 8px rgba(232, 165, 90, 0.15);
}

.order-info-meta {
  display: flex;
  gap: var(--spacing-md);
  font-size: 13px;
  align-items: center;
}

.order-info-meta .order-id {
  color: var(--colors-body);
}

.order-info-meta .timer-rem {
  color: var(--colors-muted);
}

.order-action-btn {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.order-action-btn .price {
  font-size: 13px;
  font-weight: 600;
  color: var(--colors-primary);
}

.small-pay-btn {
  background-color: var(--colors-accent-amber);
  color: #fff;
  font-size: 11px;
  font-weight: 600;
  padding: 4px 10px;
  border-radius: var(--rounded-sm);
}

.small-pay-btn:hover {
  background-color: #d97706;
}

/* Right side Technical Console */
.tech-console {
  position: fixed;
  top: 64px;
  right: 0;
  bottom: 0;
  width: 420px;
  background: rgba(24, 23, 21, 0.95);
  backdrop-filter: blur(12px);
  border-left: 1px solid var(--colors-surface-dark-soft);
  z-index: 90;
  display: flex;
  flex-direction: column;
  color: var(--colors-on-dark);
  box-shadow: -5px 0 25px rgba(0, 0, 0, 0.25);
  transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.console-header {
  height: 56px;
  border-bottom: 1px solid var(--colors-surface-dark-elevated);
  padding: 0 var(--spacing-md);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.console-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.console-dot-indicator {
  width: 8px;
  height: 8px;
  background-color: var(--colors-success);
  border-radius: var(--rounded-full);
  box-shadow: 0 0 6px var(--colors-success);
}

.console-header h3 {
  color: var(--colors-on-dark);
  font-family: var(--font-sans);
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 0.5px;
  text-transform: uppercase;
}

.console-reset-btn {
  background-color: var(--colors-error);
  color: #fff;
  font-size: 11px;
  font-weight: 600;
  padding: 5px 10px;
  border-radius: var(--rounded-sm);
}

.console-reset-btn:hover {
  background-color: #b91c1c;
}

.console-scroll-content {
  flex: 1;
  overflow-y: auto;
  padding: var(--spacing-md);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

/* 4 Grid metrics in console */
.console-metrics-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--spacing-sm);
}

.metric-card {
  background-color: var(--colors-surface-dark-soft);
  border: 1px solid var(--colors-surface-dark-elevated);
  border-radius: var(--rounded-md);
  padding: var(--spacing-sm) var(--spacing-md);
  display: flex;
  flex-direction: column;
}

.metric-card .title {
  font-size: 11px;
  color: var(--colors-on-dark-soft);
}

.metric-card .value {
  font-size: 26px;
  font-family: var(--font-serif);
  font-weight: 600;
  margin: 2px 0;
}

.metric-card .value.valkey { color: var(--colors-accent-amber); }
.metric-card .value.delay { color: var(--colors-accent-teal); }
.metric-card .value.postgres { color: var(--colors-success); }
.metric-card .value.revenue { color: var(--colors-primary); }

.metric-card .unit {
  font-size: 10px;
  color: var(--colors-on-dark-soft);
}

/* CLI Journal style */
.cli-journal-card {
  display: flex;
  flex-direction: column;
}

.cli-header {
  height: 32px;
  background-color: var(--colors-surface-dark-elevated);
  border-radius: var(--rounded-md) var(--rounded-md) 0 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 var(--spacing-sm);
  border: 1px solid var(--colors-surface-dark-soft);
  border-bottom: none;
}

.cli-dots {
  display: flex;
  gap: 4px;
}

.cli-dot {
  width: 8px;
  height: 8px;
  border-radius: var(--rounded-full);
}

.cli-dot.r { background-color: var(--colors-error); }
.cli-dot.y { background-color: var(--colors-accent-amber); }
.cli-dot.g { background-color: var(--colors-success); }

.cli-title {
  color: var(--colors-on-dark-soft);
  font-family: var(--font-mono);
  font-size: 11px;
}

.cli-terminal {
  background-color: #0d0c0b;
  border: 1px solid var(--colors-surface-dark-soft);
  border-radius: 0 0 var(--rounded-md) var(--rounded-md);
  padding: var(--spacing-sm);
  font-family: var(--font-mono);
  font-size: 11px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-height: 180px;
  max-height: 250px;
  overflow-y: auto;
  box-shadow: inset 0 2px 8px rgba(0, 0, 0, 0.5);
}

.log-row {
  line-height: 1.4;
  display: flex;
  gap: 6px;
  word-break: break-all;
}

.log-time {
  color: var(--colors-on-dark-soft);
  opacity: 0.6;
}

.log-type {
  font-weight: 600;
  padding: 0 3px;
  border-radius: var(--rounded-xs);
  font-size: 10px;
}

.log-type.info {
  background-color: rgba(93, 184, 166, 0.15);
  color: var(--colors-accent-teal);
}

.log-type.warn {
  background-color: rgba(232, 165, 90, 0.15);
  color: var(--colors-accent-amber);
}

.log-type.success {
  background-color: rgba(93, 184, 114, 0.15);
  color: var(--colors-success);
}

.log-type.error {
  background-color: rgba(198, 69, 69, 0.15);
  color: var(--colors-error);
}

.log-msg {
  color: var(--colors-on-dark);
}

.empty-log {
  color: var(--colors-on-dark-soft);
  font-style: italic;
}

.out-of-stock-alert {
  border-top: 1px dashed rgba(198, 69, 69, 0.2);
  padding-top: var(--spacing-xs);
  color: var(--colors-error);
}

/* Tech Stack explanation card */
.tech-stack-card {
  background-color: var(--colors-surface-dark-soft);
  border: 1px solid var(--colors-surface-dark-elevated);
  border-radius: var(--rounded-md);
  padding: var(--spacing-md);
}

.tech-stack-card h4 {
  font-size: 13px;
  color: var(--colors-on-dark);
  font-family: var(--font-sans);
  font-weight: 600;
  margin-bottom: 12px;
}

.principles-list {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.principles-list li {
  display: flex;
  gap: 10px;
}

.principles-list .num {
  width: 18px;
  height: 18px;
  background-color: var(--colors-surface-dark-elevated);
  color: var(--colors-accent-amber);
  border-radius: var(--rounded-full);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: bold;
  flex-shrink: 0;
}

.principles-list p {
  font-size: 11px;
  color: var(--colors-on-dark-soft);
  line-height: 1.45;
}

.principles-list strong {
  color: var(--colors-on-dark);
}

/* Animations transitions */
.fade-enter-active, .fade-leave-active {
  transition: opacity 0.3s ease;
}
.fade-enter-from, .fade-leave-to {
  opacity: 0;
}

.modal-fade-enter-active, .modal-fade-leave-active {
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}
.modal-fade-enter-from, .modal-fade-leave-to {
  opacity: 0;
  transform: scale(0.95);
}

/* Keyframes */
@keyframes pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.6; transform: scale(0.95); }
}

@keyframes pulse-border {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

/* Console Slide Animation */
.console-slide-enter-active, .console-slide-leave-active {
  transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}
.console-slide-enter-from, .console-slide-leave-to {
  transform: translateX(100%);
}

.spinner {
  width: 20px;
  height: 20px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* Responsive styles */
@media (max-width: 1200px) {
  .console-layout .product-view {
    margin-right: 0; /* Cover on smaller screens */
  }
}

@media (max-width: 768px) {
  .product-container {
    grid-template-columns: 1fr;
    gap: var(--spacing-lg);
  }
  
  .tech-console {
    width: 100%;
    top: 64px;
    height: calc(100vh - 64px);
  }
}
</style>
