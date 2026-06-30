<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import Card from '../components/ui/Card.vue'
import Input from '../components/ui/Input.vue'
import Button from '../components/ui/Button.vue'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const username = ref('')
const password = ref('')
const loading = ref(false)
const errorMsg = ref('')

const handleLogin = async () => {
  if (!username.value || !password.value) {
    errorMsg.value = '请填写用户名和密码'
    return
  }

  loading.value = true
  errorMsg.value = ''

  // Simulating api auth delay
  setTimeout(() => {
    loading.value = false
    // Simulate generic successful login
    const mockToken = 'mock-jwt-token-' + Math.random().toString(36).substring(2)
    authStore.login(username.value, mockToken)

    const redirectPath = (route.query.redirect as string) || '/'
    router.push(redirectPath)
  }, 800)
}
</script>

<template>
  <div class="login-wrapper">
    <Card variant="cream" class="login-card">
      <div class="login-header">
        <svg class="spike-mark" width="28" height="28" viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 2L14.8 9.2L22 12L14.8 14.8L12 22L9.2 14.8L2 12L9.2 9.2Z" />
        </svg>
        <h2 class="login-title">登录 GoShop</h2>
        <p class="login-subtitle">探索温和极简的企业级高并发商城</p>
      </div>

      <form @submit.prevent="handleLogin" class="login-form">
        <div class="form-group">
          <label for="username" class="form-label">用户名</label>
          <Input
            id="username"
            v-model="username"
            placeholder="请输入用户名"
            required
          />
        </div>

        <div class="form-group">
          <label for="password" class="form-label">密码</label>
          <Input
            id="password"
            v-model="password"
            type="password"
            placeholder="请输入密码"
            required
          />
        </div>

        <div v-if="errorMsg" class="error-message">
          {{ errorMsg }}
        </div>

        <Button type="submit" :loading="loading" class="submit-btn">
          进入平台
        </Button>
      </form>
    </Card>
  </div>
</template>

<style scoped>
.login-wrapper {
  min-height: 100vh;
  display: flex;
  justify-content: center;
  align-items: center;
  background-color: var(--colors-canvas);
  padding: var(--spacing-lg);
}

.login-card {
  width: 100%;
  max-width: 420px;
  padding: var(--spacing-xxl) var(--spacing-xl) !important;
}

.login-header {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  margin-bottom: var(--spacing-xl);
}

.spike-mark {
  color: var(--colors-primary);
  margin-bottom: var(--spacing-sm);
}

.login-title {
  font-family: var(--font-serif);
  font-size: 28px;
  margin-bottom: var(--spacing-xs);
}

.login-subtitle {
  font-size: 14px;
  color: var(--colors-muted);
}

.login-form {
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

.error-message {
  color: var(--colors-error);
  font-size: 13px;
  text-align: center;
}

.submit-btn {
  width: 100%;
  margin-top: var(--spacing-sm);
}
</style>
