import { api } from './api'
import type { Receipt } from './escpos'
import { loadSettings } from './sales'

// 'browser' = cetak biasa lewat browser ke printer yang terpasang di komputer ini (bawaan; tanpa penyiapan khusus).
// 'serial' dan 'usb' = browser menulis perintah ESC/POS langsung ke printer; butuh https dan izin perangkat.
// 'server' = printer menempel di server; browser menitipkan byte lewat /api/print.
export type Transport = 'browser' | 'serial' | 'usb' | 'server'

export interface PrinterConfig {
  transport: Transport
  baudRate: number
}

const KEY = 'printer'
const PRINTER_CLASS = 7

let port: SerialPort | undefined
let device: USBDevice | undefined
let endpoint = 0

// Pengaturan disimpan per perangkat kasir, karena tiap kasir bisa punya printer yang berbeda.
export function printerConfig(): PrinterConfig {
  try {
    return { transport: 'browser', baudRate: 9600, ...JSON.parse(localStorage.getItem(KEY) ?? '{}') }
  } catch {
    return { transport: 'browser', baudRate: 9600 }
  }
}

export async function savePrinterConfig(config: PrinterConfig) {
  localStorage.setItem(KEY, JSON.stringify(config))
  await forget()
}

async function forget() {
  await port?.close().catch(() => {})
  await device?.close().catch(() => {})
  port = undefined
  device = undefined
}

// Browser hanya membuka akses perangkat USB/serial di halaman https atau localhost, dan hanya Chrome/Edge yang punya.
export function supported(transport: Transport) {
  if (transport === 'serial') return 'serial' in navigator
  if (transport === 'usb') return 'usb' in navigator
  return true
}

async function openSerial(ask: boolean, baudRate: number) {
  if (port) return
  const granted = (await navigator.serial.getPorts())[0]
  if (!granted && !ask) throw new Error('Printer belum dihubungkan. Buka menu Printer.')
  const picked = granted ?? (await navigator.serial.requestPort())
  await picked.open({ baudRate })
  port = picked
}

async function openUsb(ask: boolean) {
  if (device) return
  const granted = (await navigator.usb.getDevices())[0]
  if (!granted && !ask) throw new Error('Printer belum dihubungkan. Buka menu Printer.')
  const picked = granted ?? (await navigator.usb.requestDevice({ filters: [{ classCode: PRINTER_CLASS }] }))
  await picked.open()
  if (!picked.configuration) await picked.selectConfiguration(1)
  const iface = picked.configuration?.interfaces.find((i) => i.alternate.interfaceClass === PRINTER_CLASS)
  const out = iface?.alternate.endpoints.find((e) => e.direction === 'out')
  if (!iface || !out) throw new Error('Perangkat USB ini bukan printer.')
  await picked.claimInterface(iface.interfaceNumber)
  endpoint = out.endpointNumber
  device = picked
}

// Harus dipanggil dari klik tombol: browser hanya menampilkan dialog pilih perangkat saat ada aksi pengguna.
// Cukup sekali per perangkat; setelah itu izinnya diingat dan mencetak tidak memunculkan dialog apa pun.
export async function pairPrinter() {
  const { transport, baudRate } = printerConfig()
  if (!supported(transport)) throw new Error('Browser ini tidak mengizinkan akses printer. Pakai Chrome/Edge lewat alamat https.')
  await forget()
  if (transport === 'serial') await openSerial(true, baudRate)
  if (transport === 'usb') await openUsb(true)
}

export async function isPaired() {
  const { transport } = printerConfig()
  if (!supported(transport)) return false
  if (transport === 'serial') return (await navigator.serial.getPorts()).length > 0
  if (transport === 'usb') return (await navigator.usb.getDevices()).length > 0
  if (transport === 'browser') return true
  return (await loadSettings()).printer_ready
}

// Mencetak dokumen HTML lewat dialog cetak browser, dari iframe tersembunyi supaya halaman aplikasi tidak ikut tercetak.
// Dengan Chrome yang dijalankan memakai --kiosk-printing, dialognya dilewati dan langsung ke printer bawaan.
function printHtml(html: string) {
  return new Promise<void>((resolve) => {
    const frame = document.createElement('iframe')
    frame.style.cssText = 'position:fixed;right:0;bottom:0;width:0;height:0;border:0'
    frame.srcdoc = html
    frame.onload = () => {
      const view = frame.contentWindow!
      view.onafterprint = () => {
        frame.remove()
        resolve()
      }
      view.focus()
      view.print()
    }
    document.body.append(frame)
  })
}

export async function sendToPrinter(receipt: Receipt) {
  const { transport, baudRate } = printerConfig()
  if (transport === 'browser') return printHtml(receipt.html())
  const bytes = receipt.bytes()
  if (!supported(transport)) throw new Error('Browser ini tidak mengizinkan akses printer. Pakai Chrome/Edge lewat alamat https.')
  if (transport === 'server') {
    if (!(await loadSettings()).printer_ready) throw new Error('Printer belum diatur di server.')
    await api<void>('/api/print', { method: 'POST', headers: { 'Content-Type': 'application/octet-stream' }, body: bytes })
    return
  }
  try {
    if (transport === 'usb') {
      await openUsb(false)
      await device!.transferOut(endpoint, bytes)
      return
    }
    await openSerial(false, baudRate)
    const writer = port!.writable!.getWriter()
    try {
      await writer.write(bytes)
    } finally {
      writer.releaseLock()
    }
  } catch (err) {
    // Printer dicabut atau dimatikan: buang sambungan lama supaya cetak berikutnya membukanya lagi.
    await forget()
    throw err
  }
}

if ('serial' in navigator) navigator.serial.addEventListener('disconnect', () => (port = undefined))
if ('usb' in navigator) navigator.usb.addEventListener('disconnect', () => (device = undefined))
