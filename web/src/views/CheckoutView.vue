<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useCartStore } from '../stores/cart'
import Card from '../components/ui/Card.vue'
import Button from '../components/ui/Button.vue'
import Input from '../components/ui/Input.vue'

const router = useRouter()
const cartStore = useCartStore()

const items = computed(() => cartStore.items)
const totalPrice = computed(() => cartStore.totalPrice)

const name = ref('')
const phone = ref('')
const address = ref('')
const loading = ref(false)
const orderCompleted = ref(false)

const handlePay = () => {
  if (!name.value || !phone.value || !address.value) {
    alert('请完整填写收货人信息')
    return
  }

  loading.value = true
  // Simulate payment processing
  setTimeout(() => {
    loading.value = false
    orderCompleted.value = true
    cartStore.clearCart() // Empty cart
    
    // Auto redirect after a delay
    setTimeout(() => {
      router.push('/')
    }, 3000)
  }, 1500)
}
</script>

<template>
  <div class="checkout-container">
    <h1 class="typography-display-sm checkout-title">确认订单并支付</h1>

    <div v-if="orderCompleted" class="success-screen">
      <svg class="success-icon" width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path>
        <polyline points="22 4 12 14.01 9 11.01"></polyline>
      </svg>
      <h2 class="success-title">支付成功！</h2>
      <p class="success-text">您的订单已成功创建并完成支付，我们将尽快为您安排发货。</p>
      <p class="redirect-hint">正在返回首页...</p>
      <Button @click="router.push('/')" variant="primary">回到首页</Button>
    </div>

    <div v-else-if="items.length === 0" class="empty-state">
      <p>没有需要结算的商品</p>
      <Button @click="router.push('/')" variant="primary">去挑选商品</Button>
    </div>

    <div v-else class="checkout-grid">
      <!-- Address Info Card -->
      <div class="form-section">
        <Card variant="cream" class="form-card">
          <h3 class="section-title">收货人信息</h3>
          <div class="checkout-form">
            <div class="form-group">
              <label for="name" class="form-label">姓名</label>
              <Input id="name" v-model="name" placeholder="请输入收货人姓名" />
            </div>

            <div class="form-group">
              <label for="phone" class="form-label">手机号码</label>
              <Input id="phone" v-model="phone" placeholder="请输入手机号码" />
            </div>

            <div class="form-group">
              <label for="address" class="form-label">详细地址</label>
              <Input id="address" v-model="address" placeholder="请输入详细收货地址" />
            </div>
          </div>
        </Card>
      </div>

      <!-- Order Items Summary Card -->
      <div class="summary-section">
        <Card variant="cream" class="summary-card">
          <h3 class="section-title">商品清单</h3>
          
          <div class="items-list">
            <div class="summary-item" v-for="item in items" :key="item.skuId">
              <span class="item-name">{{ item.spuName }} ({{ item.skuName }}) x {{ item.quantity }}</span>
              <span class="item-price">¥{{ ((item.price * item.quantity) / 100).toFixed(2) }}</span>
            </div>
          </div>

          <div class="price-summary">
            <div class="summary-row">
              <span>商品小计</span>
              <span>¥{{ (totalPrice / 100).toFixed(2) }}</span>
            </div>
            <div class="summary-row">
              <span>运费</span>
              <span>¥0.00</span>
            </div>
            <div class="summary-total-row">
              <span>应付总额</span>
              <span class="total-price">¥{{ (totalPrice / 100).toFixed(2) }}</span>
            </div>
          </div>

          <Button @click="handlePay" :loading="loading" variant="primary" class="pay-btn">
            立即支付
          </Button>
        </Card>
      </div>
    </div>
  </div>
</template>

<style scoped>
.checkout-container {
  max-width: 1200px;
  margin: 0 auto;
  padding: var(--spacing-section) var(--spacing-lg);
  width: 100%;
}

.checkout-title {
  margin-bottom: var(--spacing-xl);
}

.success-screen {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-xxl) 0;
  text-align: center;
  max-width: 500px;
  margin: 0 auto;
}

.success-icon {
  color: var(--colors-success);
}

.success-title {
  font-family: var(--font-serif);
  font-size: 28px;
}

.success-text {
  font-size: 15px;
  color: var(--colors-muted);
  line-height: 1.5;
}

.redirect-hint {
  font-size: 13px;
  color: var(--colors-muted-soft);
  margin-bottom: var(--spacing-sm);
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-xxl) 0;
  color: var(--colors-muted);
}

.checkout-grid {
  display: grid;
  grid-template-columns: 1.3fr 1fr;
  gap: var(--spacing-xl);
  align-items: start;
}

@media (max-width: 768px) {
  .checkout-grid {
    grid-template-columns: 1fr;
  }
}

.form-card, .summary-card {
  padding: var(--spacing-xl) !important;
}

.section-title {
  font-family: var(--font-serif);
  font-size: 20px;
  border-bottom: 1px solid var(--colors-hairline);
  padding-bottom: var(--spacing-sm);
  margin-bottom: var(--spacing-lg);
  color: var(--colors-ink);
}

.checkout-form {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.form-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--colors-body-strong);
}

.items-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-lg);
}

.summary-item {
  display: flex;
  justify-content: space-between;
  font-size: 14px;
  color: var(--colors-body);
}

.price-summary {
  border-top: 1px solid var(--colors-hairline-soft);
  padding-top: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.summary-row {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  color: var(--colors-muted);
}

.summary-total-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 15px;
  font-weight: 600;
  border-top: 1px solid var(--colors-hairline-soft);
  padding-top: var(--spacing-md);
  margin-top: var(--spacing-xs);
  color: var(--colors-ink);
}

.total-price {
  font-size: 22px;
  color: var(--colors-primary);
}

.pay-btn {
  width: 100%;
}
</style>
