import { api, post } from './api'

export interface User {
  id: number
  username: string
  full_name: string
  role: 'admin' | 'cashier'
}

export const session = $state<{ user: User | null; ready: boolean }>({ user: null, ready: false })

export async function loadSession() {
  session.user = await api<User>('/api/me').catch(() => null)
  session.ready = true
}

export async function login(username: string, password: string) {
  session.user = await post<User>('/api/login', { username, password })
}

export async function logout() {
  await post('/api/logout')
  session.user = null
}

window.addEventListener('unauthorized', () => (session.user = null))
