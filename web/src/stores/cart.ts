import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { signedFetch } from '../api/request'
import { useAuthStore } from './auth'

export interface CartItem {
  skuId: number
  spuId: number
  spuName: string
  skuName: string
  price: number
  quantity: number
  image: string
}

export const useCartStore = defineStore('cart', () => {
  const items = ref<CartItem[]>([])
  const selectedSkuIDs = ref<number[]>([]) // 被选中的 SKU ID 列表
  const authStore = useAuthStore()

  // Load cart from localStorage
  const storedCart = localStorage.getItem('goshop_cart')
  if (storedCart) {
    try {
      items.value = JSON.parse(storedCart)
      // 默认全选
      selectedSkuIDs.value = items.value.map(i => i.skuId)
    } catch {
      localStorage.removeItem('goshop_cart')
    }
  }

  const saveCart = () => {
    localStorage.setItem('goshop_cart', JSON.stringify(items.value))
  }

  const totalItems = computed(() => {
    return items.value.reduce((acc, item) => acc + item.quantity, 0)
  })

  const totalPrice = computed(() => {
    return items.value.reduce((acc, item) => acc + item.price * item.quantity, 0)
  })

  // 选中的商品列表
  const selectedItems = computed(() => {
    return items.value.filter(item => selectedSkuIDs.value.includes(item.skuId))
  })

  // 选中的商品总价
  const selectedTotalPrice = computed(() => {
    return selectedItems.value.reduce((acc, item) => acc + item.price * item.quantity, 0)
  })

  // 从后端云端拉取购物车
  async function fetchCloudCart() {
    if (!authStore.isLoggedIn) return
    try {
      const res = await signedFetch('/api/cart')
      if (res.ok) {
        const cloudItems = await res.json()
        items.value = cloudItems || []
        // 全选
        selectedSkuIDs.value = items.value.map(i => i.skuId)
        saveCart()
      }
    } catch (e) {
      console.warn('获取云端购物车失败: ', e)
    }
  }

  // 同步本地购物车到云端
  async function syncLocalToCloud() {
    if (!authStore.isLoggedIn || items.value.length === 0) return
    try {
      const syncList = items.value.map(i => ({ skuId: i.skuId, quantity: i.quantity }))
      await signedFetch('/api/cart/sync', {
        method: 'POST',
        body: JSON.stringify({ items: syncList })
      })
      await fetchCloudCart() // 重新拉取以云端为准
    } catch (e) {
      console.warn('同步本地购物车到云端失败: ', e)
    }
  }

  async function addToCart(product: Omit<CartItem, 'quantity'>, quantity: number = 1) {
    const existing = items.value.find(item => item.skuId === product.skuId)
    if (existing) {
      existing.quantity += quantity
    } else {
      items.value.push({ ...product, quantity })
    }
    
    // 勾选新加入的项
    if (!selectedSkuIDs.value.includes(product.skuId)) {
      selectedSkuIDs.value.push(product.skuId)
    }

    saveCart()

    // 云同步
    if (authStore.isLoggedIn) {
      try {
        await signedFetch('/api/cart', {
          method: 'POST',
          body: JSON.stringify({ skuId: product.skuId, quantity: existing ? existing.quantity : quantity })
        })
      } catch (e) {
        console.warn('加入云端购物车失败: ', e)
      }
    }
  }

  async function updateQuantity(skuId: number, quantity: number) {
    const item = items.value.find(i => i.skuId === skuId)
    if (item) {
      item.quantity = Math.max(1, quantity)
      saveCart()

      // 云同步
      if (authStore.isLoggedIn) {
        try {
          await signedFetch('/api/cart', {
            method: 'POST',
            body: JSON.stringify({ skuId, quantity: item.quantity })
          })
        } catch (e) {
          console.warn('更新云端购物车数量失败: ', e)
        }
      }
    }
  }

  async function removeFromCart(skuId: number) {
    items.value = items.value.filter(item => item.skuId !== skuId)
    selectedSkuIDs.value = selectedSkuIDs.value.filter(id => id !== skuId)
    saveCart()

    // 云同步
    if (authStore.isLoggedIn) {
      try {
        await signedFetch(`/api/cart/${skuId}`, {
          method: 'DELETE'
        })
      } catch (e) {
        console.warn('移除云端购物车失败: ', e)
      }
    }
  }

  function clearCart() {
    items.value = []
    selectedSkuIDs.value = []
    saveCart()
  }

  return {
    items,
    selectedSkuIDs,
    selectedItems,
    selectedTotalPrice,
    totalItems,
    totalPrice,
    fetchCloudCart,
    syncLocalToCloud,
    addToCart,
    updateQuantity,
    removeFromCart,
    clearCart
  }
})
