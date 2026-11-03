import { code128 } from './code128'

const ESC = 0x1b
const GS = 0x1d

type Align = 'left' | 'center' | 'right'

type Op =
  | { kind: 'line'; text: string; align: Align; bold: boolean; large: boolean }
  | { kind: 'barcode'; text: string }
  | { kind: 'image'; image: Bitmap }
  | { kind: 'cut' }
  | { kind: 'drawer' }

export interface Bitmap {
  width: number
  height: number
  png: string
  raster: string
}

// Receipt mencatat isi struk sekali, lalu bisa dikeluarkan dalam dua bentuk:
// html() untuk dicetak browser lewat printer biasa, bytes() berupa perintah ESC/POS untuk printer thermal.
export class Receipt {
  private ops: Op[] = []
  private width: number
  private state = { align: 'left' as Align, bold: false, large: false }

  constructor(width = 32) {
    this.width = width
  }

  align(to: Align) {
    this.state.align = to
    return this
  }

  bold(on: boolean) {
    this.state.bold = on
    return this
  }

  large(on: boolean) {
    this.state.large = on
    return this
  }

  line(text = '') {
    this.ops.push({ kind: 'line', text, ...this.state })
    return this
  }

  row(left: string, right: string) {
    const room = this.width - right.length - 1
    return this.line(left.slice(0, room).padEnd(room + 1) + right)
  }

  rule() {
    return this.line('-'.repeat(this.width))
  }

  // Barcode Code 128 dengan angkanya di bawah.
  barcode(text: string) {
    this.ops.push({ kind: 'barcode', text })
    return this
  }

  // Gambar hitam-putih di tengah, 8 titik per mm (printer 203 dpi).
  image(image: Bitmap) {
    this.ops.push({ kind: 'image', image })
    return this
  }

  cut() {
    this.ops.push({ kind: 'cut' })
    return this
  }

  // Hanya berlaku di bytes(); lewat browser, laci dibuka oleh pengaturan driver printer.
  openDrawer() {
    this.ops.push({ kind: 'drawer' })
    return this
  }

  bytes() {
    const buf = [ESC, 0x40]
    const sent = { align: 'left' as Align, bold: false, large: false }
    for (const op of this.ops) {
      if (op.kind === 'cut') buf.push(GS, 0x56, 0x42, 0x00)
      else if (op.kind === 'drawer') buf.push(ESC, 0x70, 0x00, 0x19, 0xfa)
      else if (op.kind === 'image') {
        const { width, height, raster } = op.image
        const bytesPerRow = width / 8
        buf.push(ESC, 0x61, 1, GS, 0x76, 0x30, 0, bytesPerRow & 0xff, bytesPerRow >> 8, height & 0xff, height >> 8)
        buf.push(...Uint8Array.from(atob(raster), (ch) => ch.charCodeAt(0)))
        buf.push(ESC, 0x61, { left: 0, center: 1, right: 2 }[sent.align])
      } else if (op.kind === 'barcode') {
        const data = [0x7b, 0x42, ...[...op.text].map((ch) => ch.charCodeAt(0) & 0x7f)]
        buf.push(ESC, 0x61, 1, GS, 0x68, 60, GS, 0x77, 2, GS, 0x48, 2, GS, 0x6b, 73, data.length, ...data)
        buf.push(ESC, 0x61, { left: 0, center: 1, right: 2 }[sent.align])
      } else {
        if (op.align !== sent.align) buf.push(ESC, 0x61, { left: 0, center: 1, right: 2 }[op.align])
        if (op.bold !== sent.bold) buf.push(ESC, 0x45, op.bold ? 1 : 0)
        if (op.large !== sent.large) buf.push(GS, 0x21, op.large ? 0x11 : 0x00)
        Object.assign(sent, { align: op.align, bold: op.bold, large: op.large })
        for (const ch of op.text) {
          const code = ch.charCodeAt(0)
          buf.push(code < 0x80 ? code : 0x3f)
        }
        buf.push(0x0a)
      }
    }
    return new Uint8Array(buf)
  }

  // Dokumen HTML utuh: huruf monospace 1,5 mm per karakter, jadi 32 kolom = 48 mm (kertas 57 mm)
  // dan 48 kolom = 72 mm (kertas 80 mm). Setiap cut() memulai halaman baru.
  // Ukuran halaman ditulis di @page; tanpa itu browser memakai kertas bawaan driver (sering A4/Letter)
  // dan struk dicetak mengecil. Tinggi halaman = potongan tertinggi, ditaksir dari jumlah baris.
  html() {
    const escape = (text: string) => text.replaceAll('&', '&amp;').replaceAll('<', '&lt;')
    const line = 2.5 * 1.3
    const pages = [{ html: '', height: 0 }]
    for (const op of this.ops) {
      const last = pages[pages.length - 1]
      if (op.kind === 'cut') pages.push({ html: '', height: 0 })
      else if (op.kind === 'barcode') {
        last.html += barcodeSvg(op.text) + `<div class="l c">${escape(op.text)}</div>`
        last.height += 12 + line
      } else if (op.kind === 'image') {
        last.html += `<img src="${op.image.png}" style="width:${op.image.width / 8}mm">`
        last.height += op.image.height / 8 + 1
      } else if (op.kind === 'line') {
        const classes = ['l', op.align[0], op.bold ? 'b' : '', op.large ? 'x' : ''].join(' ')
        last.html += `<div class="${classes}">${escape(op.text) || '&nbsp;'}</div>`
        last.height += op.large ? 2 * 2.5 * 1.15 : line
      }
    }
    const printed = pages.filter((page) => page.html !== '')
    const body = printed.map((page) => `<section>${page.html}</section>`).join('')
    const paper = this.width <= 32 ? 57 : 80
    // Halaman yang lebih pendek dari lebarnya dianggap landscape dan diputar oleh browser/driver.
    const height = Math.ceil(Math.max(...printed.map((page) => page.height), paper) + 7 + 3)
    return `<!doctype html><meta charset="utf-8"><style>
@page { size: ${paper}mm ${height}mm; margin: 0 }
body { margin: 0 auto; background: #fff; width: ${this.width * 1.5}mm; font: 2.5mm/1.3 'Courier New', Courier, monospace; color: #000 }
section { padding: 2mm 0 5mm; break-after: page }
section:last-child { break-after: auto }
.l { white-space: pre; overflow: hidden }
.c { text-align: center } .r { text-align: right }
.b { font-weight: bold } .x { font-size: 200%; line-height: 1.15 }
svg { display: block; width: 86%; height: 11mm; margin: 1mm auto 0 }
img { display: block; margin: 0 auto 1mm; image-rendering: pixelated }
</style>${body}`
  }
}

function barcodeSvg(text: string) {
  let x = 0
  let bars = ''
  code128(text).forEach((width, i) => {
    if (i % 2 === 0) bars += `<rect x="${x}" width="${width}" height="1"/>`
    x += width
  })
  return `<svg viewBox="0 0 ${x} 1" preserveAspectRatio="none" shape-rendering="crispEdges">${bars}</svg>`
}
