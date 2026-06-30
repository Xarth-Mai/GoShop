import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

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

  // Load cart from localStorage
  const storedCart = localStorage.getItem('goshop_cart')
  if (storedCart) {
    try {
      items.value = JSON.parse(storedCart)
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

  function addToCart(product: Omit<CartItem, 'quantity'>, quantity: number = 1) {
    const existing = items.value.find(item => item.skuId === product.skuId)
    if (existing) {
      existing.quantity += quantity
    } else {
      items.value.push({ ...product, quantity })
    }
    saveCart()
  }

  function updateQuantity(skuId: number, quantity: number) {
    const item = items.value.find(i => i.skuId === skuId)
    if (item) {
      item.quantity = Math.max(1, quantity)
      saveCart()
    }
  }

  function removeFromCart(skuId: number) {
    items.value = items.value.filter(item => item.skuId !== skuId)
    saveCart()
  }

  function clearCart() {
    items.value = []
    saveCart()
  }

  return {
    items,
    totalItems,
    totalPrice,
    addToCart,
    updateQuantity,
    removeFromCart,
    clearCart
  }
})
