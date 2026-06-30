import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export interface User {
  username: string
  token: string
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)

  // Initialize from localStorage
  const storedUser = localStorage.getItem('goshop_user')
  if (storedUser) {
    try {
      user.value = JSON.parse(storedUser)
    } catch {
      localStorage.removeItem('goshop_user')
    }
  }

  const isLoggedIn = computed(() => user.value !== null)
  const token = computed(() => user.value?.token || '')

  function login(username: string, tokenVal: string) {
    user.value = { username, token: tokenVal }
    localStorage.setItem('goshop_user', JSON.stringify(user.value))
  }

  function logout() {
    user.value = null
    localStorage.removeItem('goshop_user')
  }

  return {
    user,
    isLoggedIn,
    token,
    login,
    logout
  }
})
