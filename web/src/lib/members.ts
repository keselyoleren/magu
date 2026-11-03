import { api } from './api'

export interface Member {
  id: number
  code: string
  name: string
  phone: string
  email: string
  address: string
  bank_name: string
  bank_account_number: string
  bank_account_name: string
  commission_bps: number
  is_active: boolean
  drive_folder_id: string
}

export const listMembers = () => api<Member[]>('/api/members')

export const driveFolderUrl = (id: string) => `https://drive.google.com/drive/folders/${id}`
