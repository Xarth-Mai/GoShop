<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'

// 菜单活动状态
const activeTab = ref('dashboard')

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

const activeLog = ref('')

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
    }
  } catch (err) {
    console.error('获取指标失败:', err)
  }
}

// 模拟秒杀下单
const handleTestDecrement = async () => {
  try {
    const res = await fetch('/api/seckill', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      }
    })
    const data = await res.json()
    if (res.ok) {
      // 成功后立即拉取一次最新状态
      await fetchMetrics()
    } else {
      activeLog.value = data.message || '秒杀失败'
      setTimeout(() => {
        activeLog.value = ''
      }, 3000)
    }
  } catch (err) {
    console.error('秒杀接口请求失败:', err)
  }
}

// 重置库存
const handleReset = async () => {
  try {
    const res = await fetch('/api/reset', {
      method: 'POST'
    })
    if (res.ok) {
      await fetchMetrics()
    }
  } catch (err) {
    console.error('重置库存失败:', err)
  }
}

let timer: any = null
onMounted(() => {
  fetchMetrics()
  timer = setInterval(fetchMetrics, 1500)
})

onUnmounted(() => {
  if (timer) {
    clearInterval(timer)
  }
})
</script>

<template>
  <div class="goshop-admin">
    <!-- Top Navigation -->
    <header class="top-nav">
      <div class="nav-brand">
        <!-- Claude-like 四角星芒标志 -->
        <svg class="spike-mark" width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 2L14.8 9.2L22 12L14.8 14.8L12 22L9.2 14.8L2 12L9.2 9.2Z" />
        </svg>
        <span class="brand-title">GoShop <span class="brand-subtitle">Console</span></span>
      </div>
      <div class="nav-actions">
        <span class="user-badge">
          <span class="status-indicator"></span>
          Operator: Admin
        </span>
        <button class="btn-primary signout-btn" @click="handleReset">
          重置库存
        </button>
      </div>
    </header>

    <div class="app-body">
      <!-- Sidebar -->
      <aside class="sidebar">
        <nav class="sidebar-menu">
          <a 
            href="#" 
            class="menu-item" 
            :class="{ active: activeTab === 'dashboard' }"
            @click.prevent="activeTab = 'dashboard'"
          >
            <svg class="menu-icon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect x="3" y="3" width="7" height="9" />
              <rect x="14" y="3" width="7" height="5" />
              <rect x="14" y="12" width="7" height="9" />
              <rect x="3" y="16" width="7" height="5" />
            </svg>
            概览看板
          </a>
          <a 
            href="#" 
            class="menu-item" 
            :class="{ active: activeTab === 'spu' }"
            @click.prevent="activeTab = 'spu'"
          >
            <svg class="menu-icon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M6 2L3 6v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6l-3-4z" />
              <line x1="3" y1="6" x2="21" y2="6" />
              <path d="M16 10a4 4 0 0 1-8 0" />
            </svg>
            商品 SPU 管理
            <span class="badge-coral new-tag">Beta</span>
          </a>
          <a 
            href="#" 
            class="menu-item" 
            :class="{ active: activeTab === 'sku' }"
            @click.prevent="activeTab = 'sku'"
          >
            <svg class="menu-icon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect x="2" y="7" width="20" height="14" rx="2" ry="2" />
              <path d="M16 21V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16" />
            </svg>
            库存 SKU 管理
          </a>
          <a 
            href="#" 
            class="menu-item" 
            :class="{ active: activeTab === 'order' }"
            @click.prevent="activeTab = 'order'"
          >
            <svg class="menu-icon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="9" cy="21" r="1" />
              <circle cx="20" cy="21" r="1" />
              <path d="M1 1h4l2.68 13.39a2 2 0 0 0 2 1.61h9.72a2 2 0 0 0 2-1.61L23 6H6" />
            </svg>
            订单发货
          </a>
        </nav>

        <div class="sidebar-footer">
          <div class="system-status">
            <span class="status-dot success"></span>
            Valkey Service Active
          </div>
          <p class="footer-meta">GoShop Engine v1.0</p>
        </div>
      </aside>

      <!-- Main Content -->
      <main class="main-content">
        <!-- Dashboard Tab -->
        <div v-if="activeTab === 'dashboard'" class="tab-pane">
          <div class="dashboard-header">
            <h1 class="typography-display-md page-title">高并发场景运营监控看板</h1>
            <p class="typography-body-md page-desc">
              实时展示高性能 **Valkey (Redis)** 缓存原子扣减库存指标与延迟队列超时监控。
            </p>
          </div>

          <!-- Cards Grid (3-Up) -->
          <div class="metrics-grid">
            <!-- Card 1 -->
            <div class="feature-card">
              <div class="card-header">
                <span class="card-icon valkey-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/>
                  </svg>
                </span>
                <span class="typography-caption-uppercase category-label">内存级原子库存</span>
              </div>
              <h3 class="typography-title-md card-title">Valkey 缓存库存余量</h3>
              <div class="metric-value-container">
                <span class="metric-number">{{ metrics.seckillStock }}</span>
                <span class="metric-unit">SKU 件</span>
              </div>
              <p class="typography-body-sm card-footer-desc">
                已在内存层防止超卖，支持高并发秒杀原子性。
              </p>
            </div>

            <!-- Card 2 -->
            <div class="feature-card">
              <div class="card-header">
                <span class="card-icon delay-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <circle cx="12" cy="12" r="10"/>
                    <polyline points="12 6 12 12 16 14"/>
                  </svg>
                </span>
                <span class="typography-caption-uppercase category-label">延迟任务队列</span>
              </div>
              <h3 class="typography-title-md card-title">超时锁定中订单</h3>
              <div class="metric-value-container">
                <span class="metric-number text-teal">{{ metrics.lockStock }}</span>
                <span class="metric-unit">笔</span>
              </div>
              <p class="typography-body-sm card-footer-desc">
                基于 ZSet 的 15 分钟未支付订单自动库存回收。
              </p>
            </div>

            <!-- Card 3 -->
            <div class="feature-card">
              <div class="card-header">
                <span class="card-icon finance-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <line x1="12" y1="1" x2="12" y2="23"/>
                    <path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/>
                  </svg>
                </span>
                <span class="typography-caption-uppercase category-label">实时运营数据</span>
              </div>
              <h3 class="typography-title-md card-title">今日已支付总额</h3>
              <div class="metric-value-container">
                <span class="metric-number text-coral">¥{{ metrics.revenue }}</span>
              </div>
              <p class="typography-body-sm card-footer-desc">
                已支付：<span class="highlight">{{ metrics.ordersPaid }}</span> 笔，全链路数据最终一致。
              </p>
            </div>
          </div>

          <!-- Product Chrome & Code Window (6-6 Grid) -->
          <div class="detail-grid">
            <!-- Code Window Card (Left) -->
            <div class="code-window-card">
              <div class="window-header">
                <div class="window-dots">
                  <span class="dot red"></span>
                  <span class="dot yellow"></span>
                  <span class="dot green"></span>
                </div>
                <span class="window-title">seckill_decrement.lua</span>
                <button class="btn-primary run-script-btn" @click="handleTestDecrement" :disabled="metrics.seckillStock === 0">
                  模拟秒杀下单
                </button>
              </div>
              <div class="code-content">
                <pre class="typography-code"><code><span class="line-num">1</span><span class="keyword">local</span> key = KEYS[1]
<span class="line-num">2</span><span class="keyword">local</span> change = tonumber(ARGV[1])
<span class="line-num">3</span>
<span class="line-num">4</span><span class="keyword">local</span> current = redis.call(<span class="string">'get'</span>, key)
<span class="line-num">5</span><span class="keyword">if not</span> current <span class="keyword">then</span>
<span class="line-num">6</span>    <span class="keyword">return</span> -1
<span class="line-num">7</span><span class="keyword">end</span>
<span class="line-num">8</span>
<span class="line-num">9</span><span class="keyword">local</span> current_stock = tonumber(current)
<span class="line-num">10</span><span class="keyword">if</span> current_stock &lt; change <span class="keyword">then</span>
<span class="line-num">11</span>    <span class="keyword">return</span> 0
<span class="line-num">12</span><span class="keyword">else</span>
<span class="line-num">13</span>    redis.call(<span class="string">'decrby'</span>, key, change)
<span class="line-num">14</span>    <span class="keyword">return</span> 1
<span class="line-num">15</span><span class="keyword">end</span></code></pre>
              </div>
              <div class="window-footer">
                <span class="status-text">⚡ Memory Level Execution</span>
                <span class="status-mode">Lua 5.1 (Redis Engine)</span>
              </div>
            </div>

            <!-- Log Showcase / Product Mockup Card (Right) -->
            <div class="product-mockup-card-dark">
              <div class="mockup-header">
                <span class="mockup-dot active"></span>
                <span class="mockup-title">Valkey / Redis CLI Journal</span>
              </div>
              <div class="mockup-content">
                <div class="log-row" v-for="(log, i) in logs" :key="i">
                  <span class="log-time">[{{ log.time }}]</span>
                  <span class="log-type" :class="log.type.toLowerCase()">{{ log.type }}</span>
                  <span class="log-msg">{{ log.msg }}</span>
                </div>
                <div v-if="metrics.seckillStock === 0" class="log-row error-log">
                  <span class="log-type error">OUT_OF_STOCK</span>
                  <span class="log-msg">Lua execution aborted. Insufficient stock in Valkey cache.</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- SPU / SKU Placeholder Tabs -->
        <div v-else class="tab-pane placeholder-pane">
          <div class="placeholder-card">
            <svg class="placeholder-icon animate-pulse" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
            </svg>
            <h2 class="typography-display-sm">模块构建中</h2>
            <p class="typography-body-md">
              正在与后端 Gin `{{ activeTab.toUpperCase() }}` 控制器对接路由。本模块将基于 RBAC 精细鉴权。
            </p>
            <button class="btn-secondary" @click="activeTab = 'dashboard'">
              返回看板
            </button>
          </div>
        </div>
      </main>
    </div>
  </div>
</template>

<style scoped>
.goshop-admin {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background-color: var(--colors-canvas);
  color: var(--colors-ink);
}

/* Top Nav */
.top-nav {
  height: 64px;
  background-color: var(--colors-canvas);
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
  gap: var(--spacing-sm);
  color: var(--colors-ink);
}

.spike-mark {
  color: var(--colors-primary);
}

.brand-title {
  font-family: var(--font-serif);
  font-size: 20px;
  font-weight: 600;
  letter-spacing: -0.5px;
}

.brand-subtitle {
  font-family: var(--font-sans);
  font-size: 12px;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 1px;
  background-color: var(--colors-surface-soft);
  color: var(--colors-muted);
  padding: 2px 8px;
  border-radius: var(--rounded-sm);
  margin-left: 4px;
}

.nav-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.user-badge {
  font-size: 13px;
  color: var(--colors-body);
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background-color: var(--colors-surface-soft);
  padding: 6px 12px;
  border-radius: var(--rounded-pill);
  border: 1px solid var(--colors-hairline-soft);
}

.status-indicator {
  width: 8px;
  height: 8px;
  background-color: var(--colors-success);
  border-radius: var(--rounded-full);
}

.signout-btn {
  height: 34px !important;
  font-size: 13px !important;
  padding: 0 14px !important;
}

/* App Body Layout */
.app-body {
  flex: 1;
  display: flex;
}

/* Sidebar */
.sidebar {
  width: 260px;
  border-right: 1px solid var(--colors-hairline);
  background-color: var(--colors-canvas);
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  padding: var(--spacing-lg) var(--spacing-md);
}

.sidebar-menu {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.menu-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: 10px 14px;
  color: var(--colors-body);
  border-radius: var(--rounded-md);
  font-size: 14px;
  font-weight: 500;
  transition: all 0.2s ease;
  text-decoration: none;
}

.menu-item:hover {
  background-color: var(--colors-surface-soft);
  color: var(--colors-ink);
  text-decoration: none;
}

.menu-item.active {
  background-color: var(--colors-surface-card);
  color: var(--colors-ink);
}

.menu-icon {
  color: var(--colors-muted);
  transition: color 0.2s ease;
}

.menu-item.active .menu-icon,
.menu-item:hover .menu-icon {
  color: var(--colors-primary);
}

.new-tag {
  font-size: 10px !important;
  padding: 1px 6px !important;
  margin-left: auto;
}

.sidebar-footer {
  border-top: 1px solid var(--colors-hairline-soft);
  padding-top: var(--spacing-md);
}

.system-status {
  font-size: 12px;
  font-weight: 500;
  color: var(--colors-body);
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 4px;
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: var(--rounded-full);
}

.status-dot.success {
  background-color: var(--colors-success);
}

.footer-meta {
  font-size: 11px;
  color: var(--colors-muted-soft);
}

/* Main Content */
.main-content {
  flex: 1;
  padding: var(--spacing-xl);
  max-width: 1200px;
  margin: 0 auto;
  width: 100%;
}

.dashboard-header {
  margin-bottom: var(--spacing-xl);
}

.page-title {
  margin-bottom: var(--spacing-xs);
}

.page-desc {
  color: var(--colors-muted);
}

/* Metrics Grid */
.metrics-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--spacing-lg);
  margin-bottom: var(--spacing-xl);
}

.feature-card {
  background-color: var(--colors-surface-card);
  border-radius: var(--rounded-lg);
  padding: var(--spacing-xl);
  display: flex;
  flex-direction: column;
  border: 1px solid transparent;
  transition: all 0.2s ease;
}

.feature-card:hover {
  border-color: var(--colors-hairline);
  transform: translateY(-2px);
}

.card-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-md);
}

.card-icon {
  width: 36px;
  height: 36px;
  border-radius: var(--rounded-md);
  display: flex;
  align-items: center;
  justify-content: center;
}

.valkey-icon {
  background-color: rgba(204, 120, 92, 0.1);
  color: var(--colors-primary);
}

.delay-icon {
  background-color: rgba(93, 184, 166, 0.1);
  color: var(--colors-accent-teal);
}

.finance-icon {
  background-color: rgba(232, 165, 90, 0.1);
  color: var(--colors-accent-amber);
}

.category-label {
  color: var(--colors-muted);
}

.card-title {
  margin-bottom: var(--spacing-sm);
  font-size: 16px;
  font-weight: 600;
  color: var(--colors-body-strong);
}

.metric-value-container {
  margin-bottom: var(--spacing-md);
  display: flex;
  align-items: baseline;
  gap: 4px;
}

.metric-number {
  font-family: var(--font-serif);
  font-size: 40px;
  font-weight: 500;
  color: var(--colors-ink);
}

.metric-unit {
  font-size: 14px;
  color: var(--colors-muted);
}

.text-teal {
  color: var(--colors-accent-teal) !important;
}

.text-coral {
  color: var(--colors-primary) !important;
}

.card-footer-desc {
  border-top: 1px solid var(--colors-hairline);
  padding-top: var(--spacing-sm);
  margin-top: auto;
}

.highlight {
  font-weight: 600;
  color: var(--colors-ink);
}

/* Detail Grid (6-6 Grid) */
.detail-grid {
  display: grid;
  grid-template-columns: 1.1fr 0.9fr;
  gap: var(--spacing-lg);
}

/* Code Window Card */
.code-window-card {
  background-color: var(--colors-surface-dark);
  border-radius: var(--rounded-lg);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.window-header {
  height: 48px;
  background-color: var(--colors-surface-dark-elevated);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 var(--spacing-md);
  border-bottom: 1px solid var(--colors-surface-dark-soft);
}

.window-dots {
  display: flex;
  gap: 6px;
}

.dot {
  width: 10px;
  height: 10px;
  border-radius: var(--rounded-full);
}

.dot.red { background-color: var(--colors-error); }
.dot.yellow { background-color: var(--colors-accent-amber); }
.dot.green { background-color: var(--colors-success); }

.window-title {
  color: var(--colors-on-dark-soft);
  font-family: var(--font-mono);
  font-size: 13px;
}

.run-script-btn {
  height: 28px !important;
  font-size: 12px !important;
  padding: 0 10px !important;
}

.code-content {
  padding: var(--spacing-md);
  background-color: var(--colors-surface-dark-soft);
  flex: 1;
  overflow-x: auto;
}

.code-content pre {
  margin: 0;
}

.code-content code {
  color: var(--colors-on-dark);
  display: block;
}

.line-num {
  display: inline-block;
  width: 24px;
  color: var(--colors-muted-soft);
  border-right: 1px solid var(--colors-surface-dark-elevated);
  margin-right: 12px;
  text-align: right;
  padding-right: 8px;
  user-select: none;
}

.keyword { color: #f28c5a; }
.string { color: #8bb762; }

.window-footer {
  height: 36px;
  background-color: var(--colors-surface-dark-elevated);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 var(--spacing-md);
  font-size: 12px;
  color: var(--colors-on-dark-soft);
  border-top: 1px solid var(--colors-surface-dark-soft);
}

/* Log Showcase / Dark Mockup Card */
.product-mockup-card-dark {
  background-color: var(--colors-surface-dark);
  border-radius: var(--rounded-lg);
  padding: var(--spacing-md);
  display: flex;
  flex-direction: column;
}

.mockup-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  margin-bottom: var(--spacing-md);
  border-bottom: 1px solid var(--colors-surface-dark-elevated);
  padding-pb: var(--spacing-xs);
  padding-bottom: 8px;
}

.mockup-dot {
  width: 8px;
  height: 8px;
  border-radius: var(--rounded-full);
}

.mockup-dot.active {
  background-color: var(--colors-accent-teal);
  box-shadow: 0 0 8px var(--colors-accent-teal);
}

.mockup-title {
  color: var(--colors-on-dark);
  font-size: 13px;
  font-weight: 500;
}

.mockup-content {
  font-family: var(--font-mono);
  font-size: 13px;
  color: var(--colors-on-dark);
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 10px;
  overflow-y: auto;
  max-height: 280px;
}

.log-row {
  line-height: 1.4;
  display: flex;
  gap: 8px;
  word-break: break-all;
}

.log-time {
  color: var(--colors-muted-soft);
}

.log-type {
  font-weight: 600;
  padding: 0 4px;
  border-radius: var(--rounded-xs);
  font-size: 11px;
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
  color: var(--colors-on-dark-soft);
}

.error-log {
  border-top: 1px dashed var(--colors-surface-dark-elevated);
  padding-top: var(--spacing-sm);
}

/* Placeholder Pane */
.placeholder-pane {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 400px;
}

.placeholder-card {
  text-align: center;
  background-color: var(--colors-surface-card);
  padding: var(--spacing-xxl);
  border-radius: var(--rounded-lg);
  max-width: 500px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-md);
}

.placeholder-icon {
  color: var(--colors-muted);
}

/* Responsive */
@media (max-width: 1024px) {
  .metrics-grid {
    grid-template-columns: repeat(2, 1fr);
  }
  .detail-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 768px) {
  .app-body {
    flex-direction: column;
  }
  .sidebar {
    width: 100%;
    border-right: none;
    border-bottom: 1px solid var(--colors-hairline);
    padding: var(--spacing-md);
  }
  .metrics-grid {
    grid-template-columns: 1fr;
  }
}
</style>
