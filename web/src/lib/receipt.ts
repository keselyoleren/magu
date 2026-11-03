import { api } from './api'
import { Receipt } from './escpos'
import { rupiah } from './format'
import { logo } from './logo'
import { sendToPrinter } from './printer'
import { loadSettings, methodLabels, type Recap, type Transaction } from './sales'

export function buildReceipt(sale: Transaction, width: number) {
  const payments = (sale.payments ?? []).filter((p) => p.status === 'paid')
  const change = payments.reduce((sum, p) => sum + ((p.tendered ?? p.amount) - p.amount), 0)

  const r = new Receipt(width)
  r.align('center').image(logo)
  r.line(sale.invoice_no).line(new Date(sale.paid_at ?? sale.created_at).toLocaleString('id-ID')).align('left').rule()
  for (const item of sale.items ?? []) {
    r.line(item.name)
    r.row(`  ${item.qty} x ${rupiah(item.unit_price)}`, rupiah(item.line_total))
  }
  r.rule().bold(true).row('TOTAL', rupiah(sale.total)).bold(false)
  for (const p of payments) r.row(methodLabels[p.method], rupiah(p.tendered ?? p.amount))
  if (change > 0) r.row('Kembali', rupiah(change))
  r.rule().align('center').line(`Kasir: ${sale.cashier}`)
  if (sale.status === 'void') r.bold(true).line('*** DIBATALKAN ***').bold(false)
  r.line('Terima kasih')
  return r
}

const send = sendToPrinter

export async function printTest() {
  const settings = await loadSettings()
  const r = new Receipt(settings.receipt_width)
  r.align('center').bold(true).line(settings.store_name).bold(false).line('Tes cetak').rule()
  r.align('left').row('Kiri', 'Kanan').row('Harga', rupiah(15000)).line(new Date().toLocaleString('id-ID')).barcode('2900000000018')
  await send(r.cut())
}

export async function printRecap(recap: Recap) {
  const settings = await loadSettings()
  const r = new Receipt(settings.receipt_width)
  r.align('center').bold(true).line(settings.store_name).line('REKAP HARIAN').bold(false)
  r.line(new Date(recap.date).toLocaleDateString('id-ID', { dateStyle: 'full' })).align('left').rule()
  r.row('Transaksi lunas', String(recap.transactions)).row('Dibatalkan', String(recap.voided)).rule()
  r.row('Tunai', rupiah(recap.cash)).row('Debit', rupiah(recap.debit)).row('QRIS', rupiah(recap.qris))
  r.rule().bold(true).row('TOTAL', rupiah(recap.total)).bold(false)
  r.line(`Dicetak ${new Date().toLocaleString('id-ID')}`)
  await send(r.cut())
}

interface GoodsReceipt {
  id: number
  member_name: string
  note: string
  received_at: string
  received_by: string
  item_count: number
  total_qty: number
  items: { barcode: string; name: string; qty: number }[]
}

// Pegangan penitip bahwa barangnya sudah diserahkan; lewat jalur yang sama dengan struk supaya cukup satu printer.
export async function printGoodsReceipt(id: number) {
  const [settings, doc] = await Promise.all([loadSettings(), api<GoodsReceipt>(`/api/receipts/${id}`)])
  const r = new Receipt(settings.receipt_width)
  // Cetak browser memotong baris yang lebih lebar dari kertas, padahal nama barang di dokumen ini harus terbaca utuh.
  const wrapped = (text: string) => {
    for (const part of text.match(new RegExp(`.{1,${settings.receipt_width}}`, 'g')) ?? []) r.line(part)
  }
  r.align('center').bold(true).line(settings.store_name).line('TANDA TERIMA BARANG').bold(false)
  r.line(`No. ${doc.id}`).line(new Date(doc.received_at).toLocaleString('id-ID')).align('left').rule()
  r.line(`Dari    : ${doc.member_name || 'Barang toko'}`).line(`Petugas : ${doc.received_by}`)
  if (doc.note) wrapped(`Catatan : ${doc.note}`)
  r.rule()
  for (const item of doc.items) {
    wrapped(item.name)
    r.row(`  ${item.barcode}`, `${item.qty} pcs`)
  }
  r.rule().bold(true).row(`${doc.item_count} barang`, `${doc.total_qty} pcs`).bold(false)
  r.line().row('Diserahkan', 'Diterima').line().line().line().row('(          )', '(          )')
  await send(r.cut())
}

// Laci hanya dibuka pada cetakan pertama transaksi tunai, bukan saat cetak ulang.
export async function printReceipt(sale: Transaction, openDrawer: boolean) {
  const settings = await loadSettings()
  const receipt = buildReceipt(sale, settings.receipt_width)
  if (openDrawer && sale.payments?.some((p) => p.method === 'cash' && p.status === 'paid')) receipt.openDrawer()
  await send(receipt.cut())
}
