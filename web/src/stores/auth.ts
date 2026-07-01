import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export interface User {
  username: string
  token: string
  role: string
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)

  // Initialize from localStorage
  const storedUser = localStorage.getItem('goshop_user')
  if (storedUser) {
    try {
      const parsed = JSON.parse(storedUser)
      user.value = {
        username: parsed.username,
        token: parsed.token,
        role: parsed.role || 'user'
      }
    } catch {
      localStorage.removeItem('goshop_user')
    }
  }

  const isLoggedIn = computed(() => user.value !== null)
  const token = computed(() => user.value?.token || '')
  const isAdmin = computed(() => user.value?.role === 'admin')

  function login(username: string, tokenVal: string, role = 'user') {
    user.value = { username, token: tokenVal, role }
    localStorage.setItem('goshop_user', JSON.stringify(user.value))
  }

  function logout() {
    user.value = null
    localStorage.removeItem('goshop_user')
  }

  return {
    user,
    isLoggedIn,
    isAdmin,
    token,
    login,
    logout
  }
})
