<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useCartStore } from '../stores/cart'
import Card from '../components/ui/Card.vue'
import Button from '../components/ui/Button.vue'
import Input from '../components/ui/Input.vue'
import { signedFetch } from '../api/request'

const router = useRouter()
const cartStore = useCartStore()

const items = computed(() => cartStore.selectedItems)
const subtotal = computed(() => cartStore.selectedTotalPrice)

// App state
const addresses = ref<any[]>([])
const selectedAddress = ref<any | null>(null)
const userCoupons = ref<any[]>([])
const selectedCoupon = ref<any | null>(null)

const loading = ref(false)
const orderCompleted = ref(false)

// Dialogs Control
const showAddressListDialog = ref(false)
const showNewAddressDialog = ref(false)

// New address form state
const addressForm = ref({
  id: 0,
  receiverName: '',
  receiverPhone: '',
  province: '',
  city: '',
  district: '',
  detailAddress: '',
  isDefault: false
})

// Provinces linkage static data
const PROVINCES = [
  {
    name: '北京市',
    cities: [{ name: '北京市', districts: ['朝阳区', '海淀区', '东城区', '西城区', '丰台区'] }]
  },
  {
    name: '上海市',
    cities: [{ name: '上海市', districts: ['浦东新区', '黄浦区', '徐汇区', '长宁区', '静安区'] }]
  },
  {
    name: '广东省',
    cities: [
      { name: '广州市', districts: ['天河区', '越秀区', '海珠区', '白云区', '番禺区'] },
      { name: '深圳市', districts: ['南山区', '福田区', '罗湖区', '宝安区', '龙岗区'] }
    ]
  },
  {
    name: '浙江省',
    cities: [
      { name: '杭州市', districts: ['西湖区', '拱墅区', '上城区', '滨江区', '余杭区'] },
      { name: '宁波市', districts: ['海曙区', '江北区', '鄞州区'] }
    ]
  }
]

const citiesList = computed(() => {
  const prov = PROVINCES.find(p => p.name === addressForm.value.province)
  return prov ? prov.cities : []
})

const districtsList = computed(() => {
  const city = citiesList.value.find(c => c.name === addressForm.value.city)
  return city ? city.districts : []
})

// Watch province changes to clear city/district
watch(() => addressForm.value.province, () => {
  addressForm.value.city = ''
  addressForm.value.district = ''
})

// Watch city changes to clear district
watch(() => addressForm.value.city, () => {
  addressForm.value.district = ''
})

// Fetch addresses from backend
const fetchAddresses = async () => {
  try {
    const res = await signedFetch('/api/addresses')
    if (res.ok) {
      const data = await res.json()
      addresses.value = data || []
      // Auto select default or the first address
      const def = addresses.value.find(a => a.isDefault)
      selectedAddress.value = def || (addresses.value.length > 0 ? addresses.value[0] : null)
    }
  } catch (err) {
    console.error('获取地址失败:', err)
  }
}

// Fetch user coupons and compute the optimal choice
const fetchCoupons = async () => {
  try {
    const res = await signedFetch('/api/user-coupons')
    if (res.ok) {
      const data = await res.json()
      userCoupons.value = data || []
      autoSelectBestCoupon()
    }
  } catch (err) {
    console.error('获取可用优惠券失败:', err)
  }
}

// Optimal Coupon Matching algorithm
const autoSelectBestCoupon = () => {
  let bestVal = 0
  let bestCoupon = null

  for (const uc of userCoupons.value) {
    // Check min threshold
    if (subtotal.value >= uc.coupon.minAmount) {
      let discountVal = 0
      switch (uc.coupon.type) {
        case 1: // 满减
          discountVal = uc.coupon.value
          break
        case 2: // 折扣 (e.g. 90 means 90%, discount is 10%)
          discountVal = subtotal.value * (100 - uc.coupon.value) / 100
          break
        case 3: // 无门槛
          discountVal = uc.coupon.value
          break
      }

      if (discountVal > subtotal.value) {
        discountVal = subtotal.value
      }

      if (discountVal > bestVal) {
        bestVal = discountVal
        bestCoupon = uc
      }
    }
  }

  selectedCoupon.value = bestCoupon
}

// Calculate price variables
const shippingFee = computed(() => {
  return subtotal.value >= 9900 ? 0 : 1000 // 99元起包邮
})

const taxFee = computed(() => {
  return Math.round(subtotal.value * 0.05) // 5% 增值税率
})

const discountAmount = computed(() => {
  if (!selectedCoupon.value) return 0
  const cp = selectedCoupon.value.coupon
  let val = 0
  switch (cp.type) {
    case 1:
    case 3:
      val = cp.value
      break
    case 2:
      val = subtotal.value * (100 - cp.value) / 100
      break
  }
  return val > subtotal.value ? subtotal.value : val
})

const payableAmount = computed(() => {
  const total = subtotal.value + shippingFee.value + taxFee.value - discountAmount.value
  return total < 0 ? 0 : total
})

// Save address to database (transparently encrypted on GORM save)
const saveAddress = async () => {
  const f = addressForm.value
  if (!f.receiverName || !f.receiverPhone || !f.province || !f.city || !f.district || !f.detailAddress) {
    alert('请完整填写地址信息')
    return
  }

  try {
    const res = await signedFetch('/api/addresses', {
      method: 'POST',
      body: JSON.stringify({
        id: f.id,
        receiverName: f.receiverName,
        receiverPhone: f.receiverPhone,
        province: f.province,
        city: f.city,
        district: f.district,
        detailAddress: f.detailAddress,
        isDefault: f.isDefault
      })
    })

    if (res.ok) {
      showNewAddressDialog.value = false
      await fetchAddresses()
    } else {
      alert('保存地址失败')
    }
  } catch (err) {
    alert('请求异常，保存地址失败')
  }
}

// Switch address
const selectAddress = (addr: any) => {
  selectedAddress.value = addr
  showAddressListDialog.value = false
}

// Open new address dialog
const openNewAddress = () => {
  addressForm.value = {
    id: 0,
    receiverName: '',
    receiverPhone: '',
    province: '',
    city: '',
    district: '',
    detailAddress: '',
    isDefault: false
  }
  showNewAddressDialog.value = true
}

// Undergo order placement and immediate payment verification
const handlePay = async () => {
  if (!selectedAddress.value) {
    alert('请添加并选择收货地址')
    return
  }

  loading.value = true

  try {
    // 1. Create order
    const orderItems = items.value.map(i => ({ skuId: i.skuId, quantity: i.quantity }))
    const orderRes = await signedFetch('/api/orders', {
      method: 'POST',
      body: JSON.stringify({
        items: orderItems,
        addressId: selectedAddress.value.id,
        userCouponId: selectedCoupon.value ? selectedCoupon.value.id : 0
      })
    })

    const orderData = await orderRes.json()
    if (!orderRes.ok || orderData.status !== 'success') {
      alert(orderData.message || '创建订单失败')
      loading.value = false
      return
    }

    const orderId = orderData.orderId

    // 2. Call real Pay API
    const payRes = await signedFetch('/api/pay', {
      method: 'POST',
      body: JSON.stringify({ orderId })
    })

    if (payRes.ok) {
      orderCompleted.value = true
      // Local cart cleanup: remove checked items
      cartStore.items = cartStore.items.filter(item => !cartStore.selectedSkuIDs.includes(item.skuId))
      cartStore.selectedSkuIDs = []
      cartStore.saveCart()

      setTimeout(() => {
        router.push('/')
      }, 3000)
    } else {
      alert('订单创建成功但支付请求异常，请前往订单中心继续支付')
      router.push('/orders')
    }
  } catch (err) {
    alert('网络交互失败，请确认后端运行状态')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchAddresses()
  fetchCoupons()
})
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
      <p class="success-text">您的订单已通过企业级延迟防刷机制，完成高可用物理归档。</p>
      <p class="redirect-hint">正在返回首页...</p>
      <Button @click="router.push('/')" variant="primary">回到首页</Button>
    </div>

    <div v-else-if="items.length === 0" class="empty-state">
      <p>没有需要结算的商品</p>
      <Button @click="router.push('/')" variant="primary">去挑选商品</Button>
    </div>

    <div v-else class="checkout-grid">
      <!-- Address Info Section -->
      <div class="left-section">
        <!-- Address Card -->
        <Card variant="cream" class="checkout-card address-box-card">
          <div class="card-header-row">
            <h3 class="section-title-inline">收货地址</h3>
            <button class="text-link-btn" @click="showAddressListDialog = true">切换/管理地址</button>
          </div>

          <div v-if="selectedAddress" class="current-address-info">
            <div class="address-user-line">
              <span class="user-name">{{ selectedAddress.receiverName }}</span>
              <span class="user-phone">{{ selectedAddress.receiverPhone }}</span>
              <Badge variant="coral" class="default-badge" v-if="selectedAddress.isDefault">默认</Badge>
            </div>
            <p class="address-detail-line">
              {{ selectedAddress.province }} {{ selectedAddress.city }} {{ selectedAddress.district }} {{ selectedAddress.detailAddress }}
            </p>
          </div>
          <div v-else class="no-address-state" @click="openNewAddress">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <line x1="12" y1="5" x2="12" y2="19"></line>
              <line x1="5" y1="12" x2="19" y2="12"></line>
            </svg>
            <span>添加收货地址</span>
          </div>
        </Card>

        <!-- Coupon Card -->
        <Card variant="cream" class="checkout-card coupon-box-card">
          <h3 class="section-title-inline">卡券中心</h3>
          <div class="coupon-selector-row">
            <span class="selector-label">选择折扣券</span>
            <select v-model="selectedCoupon" class="coupon-dropdown-select">
              <option :value="null">不使用折扣卡券</option>
              <option
                v-for="uc in userCoupons"
                :key="uc.id"
                :value="uc"
                :disabled="subtotal < uc.coupon.minAmount"
              >
                {{ uc.coupon.name }} [门槛¥{{ (uc.coupon.minAmount/100) }}]
              </option>
            </select>
          </div>
          <p class="optimal-hint-text" v-if="selectedCoupon">
            已自动匹配最优组合，本次为您节省 ¥{{ (discountAmount/100).toFixed(2) }}
          </p>
        </Card>
      </div>

      <!-- Order Items Summary Card -->
      <div class="summary-section">
        <Card variant="cream" class="summary-card">
          <h3 class="section-title">订单清单</h3>
          
          <div class="items-list">
            <div class="summary-item" v-for="item in items" :key="item.skuId">
              <span class="item-name">{{ item.spuName }} ({{ item.skuName }}) x {{ item.quantity }}</span>
              <span class="item-price">¥{{ ((item.price * item.quantity) / 100).toFixed(2) }}</span>
            </div>
          </div>

          <div class="price-summary">
            <div class="summary-row">
              <span>商品小计</span>
              <span>¥{{ (subtotal / 100).toFixed(2) }}</span>
            </div>
            <div class="summary-row" v-if="discountAmount > 0">
              <span>卡券折减</span>
              <span class="discount-color">-¥{{ (discountAmount / 100).toFixed(2) }}</span>
            </div>
            <div class="summary-row">
              <span>配送运费</span>
              <span>¥{{ (shippingFee / 100).toFixed(2) }}</span>
            </div>
            <div class="summary-row">
              <span>增值税 (5%)</span>
              <span>¥{{ (taxFee / 100).toFixed(2) }}</span>
            </div>
            <div class="summary-total-row">
              <span>应付总额</span>
              <span class="total-price">¥{{ (payableAmount / 100).toFixed(2) }}</span>
            </div>
          </div>

          <Button @click="handlePay" :loading="loading" variant="primary" class="pay-btn">
            立即支付 (¥{{ (payableAmount / 100).toFixed(2) }})
          </Button>
        </Card>
      </div>
    </div>

    <!-- Address Selector Dialog (Overlay) -->
    <div class="modal-overlay" v-if="showAddressListDialog">
      <div class="modal-card">
        <div class="modal-header">
          <h3>选择收货地址</h3>
          <button class="close-btn" @click="showAddressListDialog = false">&times;</button>
        </div>
        <div class="modal-body address-list-container">
          <div
            v-for="addr in addresses"
            :key="addr.id"
            :class="['address-list-item', { active: selectedAddress?.id === addr.id }]"
            @click="selectAddress(addr)"
          >
            <div class="addr-meta">
              <strong>{{ addr.receiverName }}</strong> - <span>{{ addr.receiverPhone }}</span>
              <span class="mini-default-badge" v-if="addr.isDefault">默认</span>
            </div>
            <p>{{ addr.province }} {{ addr.city }} {{ addr.district }} {{ addr.detailAddress }}</p>
          </div>
          <Button @click="openNewAddress" variant="secondary" class="add-addr-modal-btn">新增收货地址</Button>
        </div>
      </div>
    </div>

    <!-- Add New Address Dialog (Overlay) -->
    <div class="modal-overlay" v-if="showNewAddressDialog">
      <div class="modal-card">
        <div class="modal-header">
          <h3>新建收货地址</h3>
          <button class="close-btn" @click="showNewAddressDialog = false">&times;</button>
        </div>
        <div class="modal-body form-modal-body">
          <div class="form-group-row">
            <div class="form-input-unit">
              <label>收货人姓名</label>
              <Input v-model="addressForm.receiverName" placeholder="收货人" />
            </div>
            <div class="form-input-unit">
              <label>联系手机</label>
              <Input v-model="addressForm.receiverPhone" placeholder="手机号" />
            </div>
          </div>

          <!-- Region Linkage Selector -->
          <div class="linkage-selectors-grid">
            <div class="selector-unit">
              <label>省份</label>
              <select v-model="addressForm.province" class="address-select-control">
                <option value="">请选择省份</option>
                <option v-for="p in PROVINCES" :key="p.name" :value="p.name">{{ p.name }}</option>
              </select>
            </div>
            <div class="selector-unit">
              <label>城市</label>
              <select v-model="addressForm.city" class="address-select-control" :disabled="!addressForm.province">
                <option value="">请选择城市</option>
                <option v-for="c in citiesList" :key="c.name" :value="c.name">{{ c.name }}</option>
              </select>
            </div>
            <div class="selector-unit">
              <label>区县</label>
              <select v-model="addressForm.district" class="address-select-control" :disabled="!addressForm.city">
                <option value="">请选择区县</option>
                <option v-for="d in districtsList" :key="d" :value="d">{{ d }}</option>
              </select>
            </div>
          </div>

          <div class="form-input-unit full-width">
            <label>详细地址</label>
            <Input v-model="addressForm.detailAddress" placeholder="详细路名、门牌号" />
          </div>

          <div class="default-checkbox-unit">
            <input type="checkbox" id="is_default" v-model="addressForm.isDefault" />
            <label for="is_default">设为默认收货地址</label>
          </div>

          <div class="form-actions-row">
            <Button @click="showNewAddressDialog = false" variant="secondary">取消</Button>
            <Button @click="saveAddress" variant="primary">确认保存</Button>
          </div>
        </div>
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

.left-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.checkout-card {
  padding: var(--spacing-xl) !important;
}

.card-header-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-md);
}

.section-title-inline {
  font-family: var(--font-serif);
  font-size: 18px;
  color: var(--colors-ink);
  margin: 0;
}

.text-link-btn {
  background: none;
  font-size: 13px;
  color: var(--colors-primary);
  font-weight: 500;
}

.text-link-btn:hover {
  text-decoration: underline;
}

.current-address-info {
  background-color: var(--colors-surface-soft);
  padding: var(--spacing-md);
  border-radius: var(--rounded-md);
}

.address-user-line {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  margin-bottom: 4px;
}

.user-name {
  font-weight: 600;
  font-size: 15px;
  color: var(--colors-ink);
}

.user-phone {
  color: var(--colors-muted);
  font-size: 14px;
}

.address-detail-line {
  font-size: 13px;
  color: var(--colors-body);
  margin: 0;
}

.no-address-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 90px;
  border: 1px dashed var(--colors-hairline);
  border-radius: var(--rounded-md);
  color: var(--colors-muted);
  cursor: pointer;
  gap: var(--spacing-xs);
  transition: all 0.2s ease;
}

.no-address-state:hover {
  border-color: var(--colors-primary);
  color: var(--colors-primary);
  background-color: var(--colors-surface-soft);
}

/* Coupon UI */
.coupon-selector-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  margin-top: var(--spacing-sm);
}

.selector-label {
  font-size: 13px;
  color: var(--colors-muted);
  font-weight: 500;
}

.coupon-dropdown-select {
  flex-grow: 1;
  padding: 8px 12px;
  border-radius: var(--rounded-md);
  border: 1px solid var(--colors-hairline);
  background-color: var(--colors-surface-soft);
  font-size: 13px;
  color: var(--colors-ink);
}

.optimal-hint-text {
  font-size: 12px;
  color: var(--colors-primary);
  margin-top: 8px;
  margin-left: 2px;
  font-weight: 500;
}

.discount-color {
  color: var(--colors-primary);
  font-weight: 600;
}

/* Modal overlays styles */
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
  max-width: 520px;
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

.address-list-container {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  max-height: 320px;
  overflow-y: auto;
}

.address-list-item {
  border: 1px solid var(--colors-hairline-soft);
  padding: var(--spacing-md);
  border-radius: var(--rounded-md);
  cursor: pointer;
  transition: all 0.2s ease;
}

.address-list-item:hover {
  background-color: var(--colors-surface-soft);
  border-color: var(--colors-muted);
}

.address-list-item.active {
  border-color: var(--colors-primary);
  background-color: rgba(242, 100, 25, 0.04);
}

.addr-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  margin-bottom: 4px;
}

.mini-default-badge {
  font-size: 10px;
  background-color: var(--colors-primary);
  color: var(--colors-on-dark);
  padding: 1px 4px;
  border-radius: 2px;
  font-weight: 600;
}

.add-addr-modal-btn {
  width: 100%;
  margin-top: var(--spacing-md);
}

/* Form Styles inside Modals */
.form-modal-body {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.form-group-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--spacing-md);
}

.form-input-unit {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.form-input-unit label, .selector-unit label {
  font-size: 12px;
  color: var(--colors-muted);
  font-weight: 500;
}

.linkage-selectors-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--spacing-sm);
}

.address-select-control {
  padding: 8px;
  border-radius: var(--rounded-md);
  border: 1px solid var(--colors-hairline);
  background-color: var(--colors-surface-soft);
  font-size: 13px;
  color: var(--colors-ink);
  width: 100%;
}

.default-checkbox-unit {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  margin-top: 4px;
}

.form-actions-row {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-md);
  margin-top: var(--spacing-md);
}

/* Original Checkout Layout Styles */
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

.summary-card {
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
