<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useCartStore } from '../stores/cart'
import { useAuthStore } from '../stores/auth'
import Card from '../components/ui/Card.vue'
import Button from '../components/ui/Button.vue'

const router = useRouter()
const cartStore = useCartStore()
const authStore = useAuthStore()

const items = computed(() => cartStore.items)
const selectedSkuIDs = computed({
  get: () => cartStore.selectedSkuIDs,
  set: (val) => { cartStore.selectedSkuIDs = val }
})

const selectedItemsCount = computed(() => {
  return cartStore.selectedItems.reduce((acc, item) => acc + item.quantity, 0)
})

const selectedTotalPrice = computed(() => cartStore.selectedTotalPrice)

// Checkbox select all/none computed helper
const isAllSelected = computed({
  get: () => items.value.length > 0 && selectedSkuIDs.value.length === items.value.length,
  set: (val) => {
    if (val) {
      cartStore.selectedSkuIDs = items.value.map(i => i.skuId)
    } else {
      cartStore.selectedSkuIDs = []
    }
  }
})

const updateQuantity = (skuId: number, qty: number) => {
  cartStore.updateQuantity(skuId, qty)
}

const removeItem = (skuId: number) => {
  cartStore.removeFromCart(skuId)
}

const handleCheckout = () => {
  if (cartStore.selectedItems.length === 0) {
    alert('请选择至少一件商品进行结算')
    return
  }
  router.push('/checkout')
}

onMounted(() => {
  if (authStore.isLoggedIn) {
    cartStore.fetchCloudCart()
  }
})
</script>

<template>
  <div class="cart-container">
    <h1 class="typography-display-sm cart-title">我的购物车</h1>

    <div v-if="items.length === 0" class="empty-cart-state">
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <circle cx="9" cy="21" r="1"></circle>
        <circle cx="20" cy="21" r="1"></circle>
        <path d="M1 1h4l2.68 13.39a2 2 0 0 0 2 1.61h9.72a2 2 0 0 0 2-1.61L23 6H6"></path>
      </svg>
      <p class="empty-text">您的购物车还是空的</p>
      <Button @click="router.push('/')" variant="primary">去挑选商品</Button>
    </div>

    <div v-else class="cart-grid">
      <!-- Cart Items List -->
      <div class="cart-items-section">
        <!-- Select All Control -->
        <div class="select-all-bar">
          <label class="checkbox-label">
            <input type="checkbox" v-model="isAllSelected" class="select-all-checkbox" />
            <span class="select-all-text">全选 ({{ selectedSkuIDs.length }}/{{ items.length }})</span>
          </label>
        </div>

        <Card variant="cream" class="cart-item-card" v-for="item in items" :key="item.skuId">
          <div class="item-layout">
            <input type="checkbox" :value="item.skuId" v-model="selectedSkuIDs" class="item-checkbox" />
            <img :src="item.image" :alt="item.spuName" class="item-img" />
            <div class="item-details">
              <h3 class="item-title" @click="router.push(`/product/${item.spuId}`)">{{ item.spuName }}</h3>
              <p class="item-sku-name">{{ item.skuName }}</p>
              <button @click="removeItem(item.skuId)" class="remove-btn">删除</button>
            </div>
            <div class="item-price-quantity">
              <span class="item-price">¥{{ (item.price / 100).toFixed(2) }}</span>
              <div class="qty-control">
                <button @click="updateQuantity(item.skuId, item.quantity - 1)" class="qty-btn" :disabled="item.quantity <= 1">-</button>
                <span class="qty-val">{{ item.quantity }}</span>
                <button @click="updateQuantity(item.skuId, item.quantity + 1)" class="qty-btn">+</button>
              </div>
            </div>
          </div>
        </Card>
      </div>

      <!-- Cart Summary Card -->
      <div class="cart-summary-section">
        <Card variant="cream" class="summary-card">
          <h3 class="summary-title">订单摘要</h3>
          <div class="summary-row">
            <span>选购商品数</span>
            <span>{{ selectedItemsCount }} 件</span>
          </div>
          <div class="summary-row">
            <span>运费</span>
            <span>满¥99包邮</span>
          </div>
          <div class="summary-total-row">
            <span>应付总额</span>
            <span class="total-price">¥{{ (selectedTotalPrice / 100).toFixed(2) }}</span>
          </div>
          <Button @click="handleCheckout" variant="primary" class="checkout-btn" :disabled="selectedItemsCount === 0">
            去结算 ({{ selectedItemsCount }})
          </Button>
        </Card>
      </div>
    </div>
  </div>
</template>

<style scoped>
.cart-container {
  max-width: 1200px;
  margin: 0 auto;
  padding: var(--spacing-section) var(--spacing-lg);
  width: 100%;
}

.cart-title {
  margin-bottom: var(--spacing-xl);
}

.empty-cart-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-xxl) 0;
  color: var(--colors-muted);
}

.empty-text {
  font-size: 15px;
}

.cart-grid {
  display: grid;
  grid-template-columns: 1.5fr 0.7fr;
  gap: var(--spacing-xl);
  align-items: start;
}

@media (max-width: 768px) {
  .cart-grid {
    grid-template-columns: 1fr;
  }
}

.select-all-bar {
  background-color: var(--colors-surface-card);
  padding: 10px 16px;
  border-radius: var(--rounded-md);
  border: 1px solid var(--colors-hairline-soft);
  display: flex;
  align-items: center;
  margin-bottom: var(--spacing-xs);
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
  color: var(--colors-ink);
}

.select-all-checkbox, .item-checkbox {
  width: 18px;
  height: 18px;
  border-radius: 4px;
  border: 1px solid var(--colors-hairline);
  accent-color: var(--colors-primary);
  cursor: pointer;
}

.cart-items-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.cart-item-card {
  padding: var(--spacing-md) !important;
}

.item-layout {
  display: flex;
  gap: var(--spacing-md);
  align-items: center;
}

@media (max-width: 600px) {
  .item-layout {
    flex-direction: column;
    align-items: flex-start;
  }
}

.item-img {
  width: 80px;
  height: 80px;
  object-fit: cover;
  border-radius: var(--rounded-md);
  border: 1px solid var(--colors-hairline);
  background-color: var(--colors-surface-soft);
}

.item-details {
  flex-grow: 1;
}

.item-title {
  font-family: var(--font-serif);
  font-size: 18px;
  font-weight: 500;
  color: var(--colors-ink);
  cursor: pointer;
}

.item-title:hover {
  color: var(--colors-primary);
  text-decoration: underline;
}

.item-sku-name {
  font-size: 13px;
  color: var(--colors-muted);
  margin-top: 4px;
}

.remove-btn {
  background: none;
  font-size: 12px;
  color: var(--colors-muted);
  margin-top: var(--spacing-sm);
}

.remove-btn:hover {
  color: var(--colors-error);
}

.item-price-quantity {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: var(--spacing-sm);
}

@media (max-width: 600px) {
  .item-price-quantity {
    flex-direction: row;
    justify-content: space-between;
    width: 100%;
    align-items: center;
    border-top: 1px solid var(--colors-hairline-soft);
    padding-top: var(--spacing-sm);
  }
}

.item-price {
  font-size: 16px;
  font-weight: 600;
  color: var(--colors-ink);
}

.qty-control {
  display: inline-flex;
  align-items: center;
  border: 1px solid var(--colors-hairline);
  border-radius: var(--rounded-md);
  overflow: hidden;
}

.qty-btn {
  background-color: var(--colors-surface-soft);
  color: var(--colors-ink);
  padding: 4px 10px;
  font-size: 14px;
  font-weight: 600;
  height: 28px;
}

.qty-btn:hover:not(:disabled) {
  background-color: var(--colors-hairline-soft);
}

.qty-btn:disabled {
  color: var(--colors-muted-soft);
  cursor: not-allowed;
}

.qty-val {
  padding: 0 var(--spacing-md);
  font-size: 13px;
  font-weight: 500;
}

/* Summary Card */
.summary-card {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.summary-title {
  font-family: var(--font-serif);
  font-size: 20px;
  border-bottom: 1px solid var(--colors-hairline);
  padding-bottom: var(--spacing-sm);
  color: var(--colors-ink);
}

.summary-row {
  display: flex;
  justify-content: space-between;
  font-size: 14px;
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
  font-size: 20px;
  color: var(--colors-primary);
}

.checkout-btn {
  width: 100%;
  margin-top: var(--spacing-sm);
}
</style>
