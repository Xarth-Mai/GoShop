<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useCartStore } from '../stores/cart'
import { useAuthStore } from '../stores/auth'
import Card from '../components/ui/Card.vue'
import Button from '../components/ui/Button.vue'
import Badge from '../components/ui/Badge.vue'
import { signedFetch } from '../api/request'

interface Sku {
  id: number
  spuId: number
  title: string
  specs: string
  price: number
  stock: number
  salesVolume: number
}

interface Product {
  id: number
  categoryId: number
  name: string
  subtitle: string
  description: string
  mainImage: string
  images: string
  detailHtml: string
  status: number
  skus: Sku[]
}

const props = defineProps<{
  id: string
}>()

const router = useRouter()
const cartStore = useCartStore()
const authStore = useAuthStore()

const product = ref<Product | null>(null)
const selectedSku = ref<Sku | null>(null)
const categories = ref<any[]>([])
const quantity = ref(1)
const loading = ref(false)
const message = ref('')
const messageType = ref<'success' | 'error' | ''>('')

// Coupons state
const coupons = ref<any[]>([])
const claimedCouponIDs = ref<number[]>([])

// Seckill-specific state
const backendStock = ref<number | null>(null)
const isSeckilling = ref(false)

const getProduct = async () => {
  const cached = (window as any).productCache?.[props.id]
  if (cached) {
    product.value = cached
    selectedSku.value = cached.skus?.[0] || null
    if (product.value?.id === 1) {
      fetchStock()
    }
    return
  }

  loading.value = true
  try {
    const res = await signedFetch(`/api/products/${props.id}`)
    if (res.ok) {
      const data = await res.json()
      product.value = data
      selectedSku.value = data.skus?.[0] || null
      
      if (product.value?.id === 1) {
        fetchStock()
      }
    }
  } catch (err) {
    console.error('获取商品详情失败:', err)
  } finally {
    loading.value = false
  }
}

const fetchCategories = async () => {
  try {
    const res = await signedFetch('/api/categories')
    if (res.ok) {
      categories.value = await res.json()
    }
  } catch (err) {
    console.error('获取分类失败:', err)
  }
}

const fetchCoupons = async () => {
  try {
    const res = await signedFetch('/api/coupons')
    if (res.ok) {
      coupons.value = await res.json()
    }
  } catch (err) {
    console.error('获取优惠券失败:', err)
  }
}

const fetchUserCoupons = async () => {
  if (!authStore.isLoggedIn) return
  try {
    const res = await signedFetch('/api/user-coupons')
    if (res.ok) {
      const data = await res.json()
      claimedCouponIDs.value = data.map((uc: any) => uc.couponId)
    }
  } catch (err) {
    console.error('获取已领卡券失败:', err)
  }
}

const isClaimed = (couponId: number) => {
  return claimedCouponIDs.value.includes(couponId)
}

const claimCoupon = async (couponId: number) => {
  if (!authStore.isLoggedIn) {
    router.push({ name: 'Login', query: { redirect: router.currentRoute.value.fullPath } })
    return
  }
  if (isClaimed(couponId)) {
    showMessage('您已经领取过此优惠券！', 'success')
    return
  }

  try {
    const res = await signedFetch('/api/user-coupons/receive', {
      method: 'POST',
      body: JSON.stringify({ couponId })
    })
    const data = await res.json()
    if (res.ok) {
      showMessage('优惠券领取成功！已存入您的卡券包', 'success')
      claimedCouponIDs.value.push(couponId)
    } else {
      showMessage(data.message || '优惠券领取失败', 'error')
    }
  } catch (err) {
    showMessage('领取请求异常，请检查服务状态', 'error')
  }
}

const categoryName = computed(() => {
  if (!product.value) return ''
  const cat = categories.value.find(c => c.id === product.value.categoryId)
  return cat ? cat.name : '商品'
})

const selectSku = (sku: Sku) => {
  selectedSku.value = sku
}

const showMessage = (msg: string, type: 'success' | 'error') => {
  message.value = msg
  messageType.value = type
  setTimeout(() => {
    message.value = ''
    messageType.value = ''
  }, 4000)
}

const fetchStock = async () => {
  if (product.value?.id !== 1) return
  try {
    const res = await signedFetch('/api/metrics')
    if (res.ok) {
      const data = await res.json()
      backendStock.value = data.metrics.seckillStock
      const sku1 = product.value.skus.find(s => s.id === 1)
      if (sku1) sku1.stock = data.metrics.seckillStock
    }
  } catch (err) {
    console.error('获取秒杀库存失败:', err)
  }
}

onMounted(() => {
  fetchCategories()
  getProduct()
  fetchCoupons()
  fetchUserCoupons()
  
  const idNum = parseInt(props.id)
  if (idNum === 1) {
    const timer = setInterval(fetchStock, 3000)
    return () => clearInterval(timer)
  }
})

// Cloud synced cart add function
const handleAddToCart = async () => {
  if (!product.value || !selectedSku.value) return
  
  // Local Pinia update
  cartStore.addToCart({
    skuId: selectedSku.value.id,
    spuId: product.value.id,
    spuName: product.value.name,
    skuName: selectedSku.value.title,
    price: selectedSku.value.price,
    image: product.value.mainImage
  }, quantity.value)

  // Cloud database sync
  if (authStore.isLoggedIn) {
    try {
      await signedFetch('/api/cart', {
        method: 'POST',
        body: JSON.stringify({
          skuId: selectedSku.value.id,
          quantity: quantity.value
        })
      })
    } catch (e) {
      console.warn('云端购物车同步失败，暂存本地: ', e)
    }
  }

  showMessage('商品已成功加入云端购物车！', 'success')
}

const handleSeckillPurchase = async () => {
  if (!authStore.isLoggedIn) {
    router.push({ name: 'Login', query: { redirect: router.currentRoute.value.fullPath } })
    return
  }

  isSeckilling.value = true
  try {
    const res = await signedFetch('/api/seckill', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      }
    })
    const data = await res.json()
    if (res.ok && data.status === 'success') {
      showMessage('抢购成功！秒杀订单已创建，请在 15 秒内完成支付。', 'success')
      setTimeout(() => {
        router.push('/orders')
      }, 1500)
    } else {
      showMessage(data.message || '秒杀抢购失败，商品已售罄或排队人数过多', 'error')
    }
  } catch (err) {
    showMessage('网络请求失败，请检查网络或服务状态', 'error')
  } finally {
    isSeckilling.value = false
  }
}

const handleStandardPurchase = () => {
  if (!authStore.isLoggedIn) {
    router.push({ name: 'Login', query: { redirect: router.currentRoute.value.fullPath } })
    return
  }
  if (!product.value || !selectedSku.value) return

  // Sync to local cart store and redirect
  cartStore.clearCart()
  cartStore.addToCart({
    skuId: selectedSku.value.id,
    spuId: product.value.id,
    spuName: product.value.name,
    skuName: selectedSku.value.title,
    price: selectedSku.value.price,
    image: product.value.mainImage
  }, quantity.value)
  
  router.push('/checkout')
}
</script>

<template>
  <div class="detail-container" v-if="product">
    <div class="product-grid">
      <!-- Image Gallery -->
      <div class="product-gallery">
        <div class="image-wrapper">
          <img :src="product.mainImage" :alt="product.name" />
        </div>
      </div>

      <!-- Product Meta -->
      <div class="product-meta">
        <div class="meta-header">
          <Badge variant="coral" v-if="product.id === 1">秒杀特惠</Badge>
          <span class="category-name">{{ categoryName }}</span>
        </div>

        <h1 class="typography-display-sm product-title">{{ product.name }}</h1>
        <p class="product-subtitle">{{ product.subtitle }}</p>

        <div class="price-box">
          <span class="price-label">售价</span>
          <span class="price-value">¥{{ ((selectedSku?.price || product.price) / 100).toFixed(2) }}</span>
        </div>

        <!-- Coupons Claim Section -->
        <div class="coupons-section" v-if="coupons.length > 0">
          <span class="coupons-label">专享卡券：</span>
          <div class="coupons-list">
            <span
              v-for="cp in coupons"
              :key="cp.id"
              :class="['coupon-badge-item', { claimed: isClaimed(cp.id) }]"
              @click="claimCoupon(cp.id)"
            >
              {{ cp.name }}
              <span class="badge-status">{{ isClaimed(cp.id) ? '已领' : '领券' }}</span>
            </span>
          </div>
        </div>

        <div class="product-desc-box">
          <p>{{ product.description }}</p>
        </div>

        <!-- Sku Selection -->
        <div class="sku-selection">
          <h4 class="selection-title">规格</h4>
          <div class="sku-options">
            <button
              v-for="sku in product.skus"
              :key="sku.id"
              @click="selectSku(sku)"
              :class="['sku-btn', { active: selectedSku?.id === sku.id }]"
            >
              {{ sku.title }}
            </button>
          </div>
        </div>

        <!-- Quantity Selection -->
        <div class="qty-selection" v-if="product.id !== 1">
          <h4 class="selection-title">数量</h4>
          <div class="qty-control">
            <button @click="quantity = Math.max(1, quantity - 1)" class="qty-btn" :disabled="quantity <= 1">-</button>
            <span class="qty-val">{{ quantity }}</span>
            <button @click="quantity++" class="qty-btn">+</button>
          </div>
        </div>

        <!-- Stock Indicator -->
        <div class="stock-info">
          <span class="stock-label">库存状态：</span>
          <span v-if="product.id === 1" class="stock-value highlight">
            Valkey 缓存余量: {{ backendStock !== null ? backendStock : '获取中...' }} 件
          </span>
          <span v-else class="stock-value">
            {{ selectedSku ? selectedSku.stock : 0 }} 件可用
          </span>
        </div>

        <!-- Feedback Messages -->
        <div v-if="message" :class="['message-box', messageType]">
          {{ message }}
        </div>

        <!-- CTA Buttons -->
        <div class="action-buttons">
          <!-- If SPU is Claude Phone (ID 1), trigger high-concurrency Seckill flow -->
          <template v-if="product.id === 1">
            <Button
              @click="handleSeckillPurchase"
              :loading="isSeckilling"
              variant="primary"
              class="seckill-cta"
              :disabled="backendStock !== null && backendStock <= 0"
            >
              {{ backendStock !== null && backendStock <= 0 ? '库存已售罄' : '立即秒杀抢购' }}
            </Button>
          </template>

          <template v-else>
            <Button @click="handleAddToCart" variant="secondary" class="cart-cta">
              加入购物车
            </Button>
            <Button @click="handleStandardPurchase" variant="primary" class="buy-cta">
              立即购买
            </Button>
          </template>
        </div>
      </div>
    </div>
  </div>
  <div v-else class="loading-state">
    正在加载商品详情...
  </div>
</template>

<style scoped>
.detail-container {
  max-width: 1200px;
  margin: 0 auto;
  padding: var(--spacing-section) var(--spacing-lg);
  width: 100%;
}

.loading-state {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 300px;
  color: var(--colors-muted);
}

.product-grid {
  display: grid;
  grid-template-columns: 1.1fr 0.9fr;
  gap: var(--spacing-xxl);
}

@media (max-width: 768px) {
  .product-grid {
    grid-template-columns: 1fr;
    gap: var(--spacing-xl);
  }
}

.product-gallery {
  width: 100%;
}

.image-wrapper {
  width: 100%;
  aspect-ratio: 1;
  background-color: var(--colors-surface-card);
  border-radius: var(--rounded-lg);
  overflow: hidden;
  border: 1px solid var(--colors-hairline);
}

.image-wrapper img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.product-meta {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.meta-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.category-name {
  font-size: 13px;
  color: var(--colors-muted);
  font-weight: 500;
}

.product-title {
  margin-top: var(--spacing-xxs);
}

.product-subtitle {
  color: var(--colors-muted);
  font-size: 16px;
  line-height: 1.4;
}

.price-box {
  background-color: var(--colors-surface-soft);
  padding: var(--spacing-md);
  border-radius: var(--rounded-md);
  display: flex;
  align-items: center;
  gap: var(--spacing-lg);
}

.price-label {
  font-size: 14px;
  color: var(--colors-muted);
}

.price-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--colors-primary);
}

/* Coupons claim styles */
.coupons-section {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) 0;
  border-bottom: 1px dashed var(--colors-hairline-soft);
}

.coupons-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--colors-muted);
  min-width: 60px;
}

.coupons-list {
  display: flex;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}

.coupon-badge-item {
  background-color: rgba(242, 100, 25, 0.08);
  border: 1px dashed var(--colors-primary);
  color: var(--colors-primary);
  font-size: 12px;
  font-weight: 500;
  padding: 4px 10px;
  border-radius: 4px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1);
}

.coupon-badge-item:hover:not(.claimed) {
  background-color: var(--colors-primary);
  color: var(--colors-on-dark);
  transform: translateY(-1px);
}

.coupon-badge-item.claimed {
  background-color: var(--colors-surface-soft);
  border-color: var(--colors-hairline);
  color: var(--colors-muted);
  cursor: not-allowed;
}

.badge-status {
  background-color: var(--colors-primary);
  color: var(--colors-on-dark);
  font-size: 9px;
  padding: 1px 4px;
  border-radius: 2px;
  font-weight: 600;
}

.coupon-badge-item.claimed .badge-status {
  background-color: var(--colors-hairline);
  color: var(--colors-muted);
}

.product-desc-box {
  font-size: 15px;
  color: var(--colors-body);
  line-height: 1.6;
}

.selection-title {
  font-family: var(--font-sans);
  font-size: 14px;
  font-weight: 600;
  color: var(--colors-ink);
  margin-bottom: var(--spacing-sm);
}

.sku-options {
  display: flex;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}

.sku-btn {
  background-color: var(--colors-canvas);
  border: 1px solid var(--colors-hairline);
  padding: 10px 16px;
  border-radius: var(--rounded-md);
  font-size: 14px;
  color: var(--colors-body-strong);
}

.sku-btn:hover {
  border-color: var(--colors-primary);
  color: var(--colors-primary);
}

.sku-btn.active {
  border-color: var(--colors-primary);
  color: var(--colors-primary);
  background-color: rgba(204, 120, 92, 0.05);
  font-weight: 500;
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
  padding: 8px 16px;
  font-size: 16px;
  font-weight: 600;
  height: 38px;
}

.qty-btn:hover:not(:disabled) {
  background-color: var(--colors-hairline-soft);
}

.qty-btn:disabled {
  color: var(--colors-muted-soft);
  cursor: not-allowed;
}

.qty-val {
  padding: 0 var(--spacing-lg);
  font-size: 15px;
  font-weight: 500;
}

.stock-info {
  font-size: 14px;
  color: var(--colors-muted);
}

.stock-value.highlight {
  color: var(--colors-accent-teal);
  font-weight: 600;
}

.message-box {
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--rounded-md);
  font-size: 14px;
  line-height: 1.4;
  margin-top: var(--spacing-xs);
}

.message-box.success {
  background-color: rgba(93, 184, 114, 0.1);
  color: var(--colors-success);
  border: 1px solid rgba(93, 184, 114, 0.2);
}

.message-box.error {
  background-color: rgba(198, 69, 69, 0.1);
  color: var(--colors-error);
  border: 1px solid rgba(198, 69, 69, 0.2);
}

.action-buttons {
  display: flex;
  gap: var(--spacing-md);
  margin-top: var(--spacing-md);
}

.seckill-cta {
  flex-grow: 1;
  height: 48px !important;
  font-size: 16px !important;
}

.cart-cta, .buy-cta {
  flex-grow: 1;
  height: 44px !important;
  font-size: 15px !important;
}
</style>
