import { api } from './api'

export interface Transaction {
  id: number
  invoice_no: string
  status: 'pending' | 'paid' | 'void'
  total: number
  created_at: string
  paid_at: string | null
  cashier: string
  methods: string
  items?: { name: string; qty: number; unit_price: number; line_total: number }[]
  payments?: {
    id: number
    method: 'cash' | 'debit' | 'qris'
    amount: number
    tendered: number | null
    reference: string
    status: 'pending' | 'paid' | 'failed' | 'expired'
    qr_string: string
  }[]
}

export interface Settings {
  store_name: string
  receipt_width: number
  printer_ready: boolean
  qris_enabled: boolean
  drive_enabled: boolean
}

export interface Recap {
  date: string
  transactions: number
  voided: number
  cash: number
  debit: number
  qris: number
  total: number
}

export const methodLabels = { cash: 'Tunai', debit: 'Debit', qris: 'QRIS' }

export const describeMethods = (methods: string) =>
  methods
    .split(',')
    .filter(Boolean)
    .map((m) => methodLabels[m as keyof typeof methodLabels] ?? m)
    .join(' + ')

let settings: Promise<Settings> | undefined
export const loadSettings = () => (settings ??= api<Settings>('/api/settings'))
