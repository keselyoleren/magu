<script lang="ts">
  import { code128 } from '../lib/code128'

  let { value }: { value: string } = $props()

  const QUIET = 10

  const bars = $derived.by(() => {
    let x = QUIET
    const rects: { x: number; width: number }[] = []
    code128(value).forEach((width, i) => {
      if (i % 2 === 0) rects.push({ x, width })
      x += width
    })
    return { rects, total: x + QUIET }
  })
</script>

<svg viewBox="0 0 {bars.total} 40" preserveAspectRatio="none" shape-rendering="crispEdges" role="img" aria-label="Barcode {value}">
  {#each bars.rects as bar}
    <rect x={bar.x} y="0" width={bar.width} height="40" />
  {/each}
</svg>

<style>
  svg {
    display: block;
    width: 100%;
    height: 100%;
  }
</style>
