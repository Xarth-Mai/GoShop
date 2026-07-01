<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import Card from '../components/ui/Card.vue'
import Input from '../components/ui/Input.vue'
import Button from '../components/ui/Button.vue'

const router = useRouter()

const username = ref('')
const email = ref('')
const password = ref('')
const confirmPassword = ref('')
const loading = ref(false)
const errorMsg = ref('')
const successMsg = ref('')

const handleRegister = async () => {
  if (!username.value || !password.value || !confirmPassword.value) {
    errorMsg.value = '请填写用户名和密码'
    return
  }
  if (password.value.length < 6) {
    errorMsg.value = '密码至少需要 6 位'
    return
  }
  if (password.value !== confirmPassword.value) {
    errorMsg.value = '两次输入的密码不一致'
    return
  }

  loading.value = true
  errorMsg.value = ''
  successMsg.value = ''

  try {
    const response = await fetch('/api/auth/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        username: username.value,
        password: password.value,
        email: email.value
      })
    })
    const data = await response.json().catch(() => ({}))
    if (!response.ok) {
      errorMsg.value = data.message || '注册失败，请稍后重试'
      return
    }

    successMsg.value = '注册成功，请登录'
    setTimeout(() => {
      router.push({ name: 'Login', query: { username: username.value } })
    }, 700)
  } catch (err) {
    console.error('Register request failed:', err)
    errorMsg.value = '无法连接注册服务，请稍后重试'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="register-wrapper">
    <Card variant="cream" class="register-card">
      <div class="register-header">
        <svg class="spike-mark" width="28" height="28" viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 2L14.8 9.2L22 12L14.8 14.8L12 22L9.2 14.8L2 12L9.2 9.2Z" />
        </svg>
        <h2 class="register-title">创建 GoShop 账号</h2>
        <p class="register-subtitle">注册后即可同步购物车、地址与订单</p>
      </div>

      <form @submit.prevent="handleRegister" class="register-form">
        <div class="form-group">
          <label for="username" class="form-label">用户名</label>
          <Input id="username" v-model="username" placeholder="请输入用户名" required />
        </div>

        <div class="form-group">
          <label for="email" class="form-label">邮箱</label>
          <Input id="email" v-model="email" type="email" placeholder="可选" />
        </div>

        <div class="form-group">
          <label for="password" class="form-label">密码</label>
          <Input id="password" v-model="password" type="password" placeholder="至少 6 位" required />
        </div>

        <div class="form-group">
          <label for="confirmPassword" class="form-label">确认密码</label>
          <Input id="confirmPassword" v-model="confirmPassword" type="password" placeholder="再次输入密码" required />
        </div>

        <div v-if="errorMsg" class="error-message">{{ errorMsg }}</div>
        <div v-if="successMsg" class="success-message">{{ successMsg }}</div>

        <Button type="submit" :loading="loading" class="submit-btn">注册账号</Button>
        <Button type="button" variant="text" class="login-link" @click="router.push('/login')">已有账号，去登录</Button>
      </form>
    </Card>
  </div>
</template>

<style scoped>
.register-wrapper {
  min-height: 100vh;
  display: flex;
  justify-content: center;
  align-items: center;
  background-color: var(--colors-canvas);
  padding: var(--spacing-lg);
}

.register-card {
  width: 100%;
  max-width: 440px;
  padding: var(--spacing-xxl) var(--spacing-xl) !important;
}

.register-header {
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

.register-title {
  font-family: var(--font-serif);
  font-size: 28px;
  margin-bottom: var(--spacing-xs);
}

.register-subtitle {
  font-size: 14px;
  color: var(--colors-muted);
}

.register-form {
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

.error-message,
.success-message {
  font-size: 13px;
  text-align: center;
}

.error-message {
  color: var(--colors-error);
}

.success-message {
  color: var(--colors-success);
}

.submit-btn {
  width: 100%;
  margin-top: var(--spacing-sm);
}

.login-link {
  align-self: center;
}
</style>
