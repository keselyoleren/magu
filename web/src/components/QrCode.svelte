<script lang="ts">
  import qrcode from 'qrcode-generator'

  let { value }: { value: string } = $props()

  const QUIET = 4

  const code = $derived.by(() => {
    const qr = qrcode(0, 'M')
    qr.addData(value)
    qr.make()
    const count = qr.getModuleCount()
    let path = ''
    for (let row = 0; row < count; row++) {
      for (let col = 0; col < count; col++) {
        if (qr.isDark(row, col)) path += `M${col + QUIET} ${row + QUIET}h1v1h-1z`
      }
    }
    return { path, size: count + QUIET * 2 }
  })
</script>

<svg viewBox="0 0 {code.size} {code.size}" shape-rendering="crispEdges" role="img" aria-label="Kode QRIS">
  <rect width={code.size} height={code.size} fill="#fff" />
  <path d={code.path} fill="#000" />
</svg>

<style>
  svg {
    display: block;
    width: 100%;
    height: auto;
    border-radius: 8px;
  }
</style>
