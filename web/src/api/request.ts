import { useAuthStore } from '../stores/auth'

// Front-end signature key (must match backend config jwt.secret)
const CLIENT_SECRET = 'your_goshop_jwt_secret_key_change_me'

// Generate random Nonce strings
function generateNonce(length: number = 18): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'
  let result = ''
  for (let i = 0; i < length; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  return result
}

// Calculate HMAC-SHA256 signature using browser native Web Crypto API
async function calculateHMAC(secret: string, message: string): Promise<string> {
  const encoder = new TextEncoder()
  const keyData = encoder.encode(secret)
  const messageData = encoder.encode(message)

  try {
    const cryptoKey = await window.crypto.subtle.importKey(
      'raw',
      keyData,
      { name: 'HMAC', hash: { name: 'SHA-256' } },
      false,
      ['sign']
    )

    const signature = await window.crypto.subtle.sign(
      'HMAC',
      cryptoKey,
      messageData
    )

    return Array.from(new Uint8Array(signature))
      .map(b => b.toString(16).padStart(2, '0'))
      .join('')
  } catch (err) {
    console.error('Front-end signature calculation error:', err)
    return ''
  }
}

// SignedFetch: Native fetch wrapper implementing authorization, rate limiting, sign authentication and replay protection
export async function signedFetch(path: string, options: RequestInit = {}): Promise<Response> {
  const authStore = useAuthStore()
  
  // Clone options and headers to avoid mutations
  const headers = new Headers(options.headers || {})
  const method = (options.method || 'GET').toUpperCase()

  // 1. JWT Authorization Header
  if (authStore.isLoggedIn && authStore.token) {
    headers.set('Authorization', `Bearer ${authStore.token}`)
  }

  // 2. Sign and Nonce Headers for POST, PUT, DELETE write operations
  if (method === 'POST' || method === 'PUT' || method === 'DELETE') {
    const timestamp = Date.now().toString()
    const nonce = generateNonce(20)
    const bodyStr = options.body ? (typeof options.body === 'string' ? options.body : JSON.stringify(options.body)) : ''

    // Sign message format: timestamp + nonce + path + body
    const message = timestamp + nonce + path + bodyStr
    const sign = await calculateHMAC(CLIENT_SECRET, message)

    headers.set('X-Timestamp', timestamp)
    headers.set('X-Nonce', nonce)
    headers.set('X-Sign', sign)
  }

  return fetch(path, {
    ...options,
    headers
  })
}
