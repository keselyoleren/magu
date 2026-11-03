export class ApiError extends Error {
  status: number
  details: string[]

  constructor(message: string, status: number, details: string[] = []) {
    super(message)
    this.status = status
    this.details = details
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init.headers },
  })
  if (res.status === 401 && path !== '/api/login') window.dispatchEvent(new Event('unauthorized'))
  if (!res.ok) {
    const body = await res.json().catch(() => null)
    throw new ApiError(body?.error ?? `Permintaan gagal (${res.status})`, res.status, body?.details)
  }
  return res.status === 204 ? (undefined as T) : res.json()
}

export const post = <T>(path: string, data?: unknown) => api<T>(path, { method: 'POST', body: JSON.stringify(data) })
export const del = (path: string) => api<void>(path, { method: 'DELETE' })
export const put = <T>(path: string, data: unknown) => api<T>(path, { method: 'PUT', body: JSON.stringify(data) })
