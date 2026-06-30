<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../../stores/auth'
import { useCartStore } from '../../stores/cart'

const router = useRouter()
const authStore = useAuthStore()
const cartStore = useCartStore()

const isLoggedIn = computed(() => authStore.isLoggedIn)
const user = computed(() => authStore.user)
const cartCount = computed(() => cartStore.totalItems)

const handleLogout = () => {
  authStore.logout()
  router.push('/login')
}
</script>

<template>
  <header class="top-nav">
    <div class="nav-container">
      <!-- Brand Logo -->
      <div class="nav-brand" @click="router.push('/')">
        <svg class="spike-mark" width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 2L14.8 9.2L22 12L14.8 14.8L12 22L9.2 14.8L2 12L9.2 9.2Z" />
        </svg>
        <span class="brand-name">GoShop</span>
      </div>

      <!-- Navigation Links -->
      <nav class="nav-menu">
        <router-link to="/" class="nav-link" active-class="active">首页</router-link>
        <router-link to="/orders" class="nav-link" active-class="active" v-if="isLoggedIn">我的订单</router-link>
      </nav>

      <!-- Action Items -->
      <div class="nav-actions">
        <!-- Cart Link -->
        <router-link to="/cart" class="cart-link">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="9" cy="21" r="1"></circle>
            <circle cx="20" cy="21" r="1"></circle>
            <path d="M1 1h4l2.68 13.39a2 2 0 0 0 2 1.61h9.72a2 2 0 0 0 2-1.61L23 6H6"></path>
          </svg>
          <span v-if="cartCount > 0" class="cart-badge">{{ cartCount }}</span>
        </router-link>

        <!-- Auth Actions -->
        <div v-if="isLoggedIn" class="user-menu">
          <span class="username">{{ user?.username }}</span>
          <button @click="handleLogout" class="logout-btn">退出</button>
        </div>
        <router-link v-else to="/login" class="login-btn-link">
          登录
        </router-link>
      </div>
    </div>
  </header>
</template>

<style scoped>
.top-nav {
  position: sticky;
  top: 0;
  z-index: 100;
  height: 64px;
  background-color: var(--colors-canvas);
  border-bottom: 1px solid var(--colors-hairline);
  display: flex;
  align-items: center;
}

.nav-container {
  width: 100%;
  max-width: 1200px;
  margin: 0 auto;
  padding: 0 var(--spacing-lg);
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.nav-brand {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  cursor: pointer;
}

.spike-mark {
  color: var(--colors-primary);
}

.brand-name {
  font-family: var(--font-serif);
  font-size: 22px;
  font-weight: 600;
  color: var(--colors-ink);
}

.nav-menu {
  display: flex;
  gap: var(--spacing-lg);
}

.nav-link {
  font-size: 14px;
  font-weight: 500;
  color: var(--colors-muted);
  transition: color 0.2s ease;
  position: relative;
  padding: 6px 0;
}

.nav-link:hover, .nav-link.active {
  color: var(--colors-ink);
  text-decoration: none;
}

.nav-link.active::after {
  content: '';
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 2px;
  background-color: var(--colors-primary);
}

.nav-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-lg);
}

.cart-link {
  color: var(--colors-muted);
  display: flex;
  align-items: center;
  position: relative;
}

.cart-link:hover {
  color: var(--colors-ink);
}

.cart-badge {
  position: absolute;
  top: -8px;
  right: -10px;
  background-color: var(--colors-primary);
  color: var(--colors-on-primary);
  font-size: 10px;
  font-weight: 700;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.user-menu {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.username {
  font-size: 14px;
  font-weight: 500;
  color: var(--colors-body-strong);
}

.logout-btn {
  background: none;
  font-size: 13px;
  color: var(--colors-muted);
}

.logout-btn:hover {
  color: var(--colors-error);
}

.login-btn-link {
  background-color: var(--colors-primary);
  color: var(--colors-on-primary);
  font-size: 14px;
  font-weight: 500;
  padding: 8px 16px;
  border-radius: var(--rounded-md);
  transition: all 0.2s ease;
}

.login-btn-link:hover {
  background-color: var(--colors-primary-active);
  text-decoration: none;
}
</style>
