<script lang="ts">
  import Icon from '../../components/Icon.svelte'
  import { api, del, post, put } from '../../lib/api'
  import { dateTime } from '../../lib/format'
  import type { Opname } from './Opnames.svelte'

  type Item = NonNullable<Opname['items']>[number]

  let { id }: { id: number } = $props()

  let opname = $state<Opname | null>(null)
  let items = $state<Item[]>([])
  let search = $state('')
  let view = $state<'all' | 'uncounted' | 'diff'>('all')
  let scan = $state('')
  let message = $state('')
  let failed = $state(false)
  let busy = $state(false)

  const isDraft = $derived(opname?.status === 'draft')
  const counted = $derived(items.filter((i) => i.physical_qty !== null).length)
  const diffs = $derived(items.filter((i) => diff(i) !== 0 && i.physical_qty !== null).length)
  const shown = $derived.by(() => {
    const q = search.trim().toLowerCase()
    return items.filter(
      (i) =>
        (!q || i.name.toLowerCase().includes(q) || i.barcode.includes(q)) &&
        (view === 'all' ||
          (view === 'uncounted' && i.physical_qty === null) ||
          (view === 'diff' && i.physical_qty !== null && diff(i) !== 0)),
    )
  })

  function diff(i: Item) {
    return (i.physical_qty ?? i.system_qty) - i.system_qty
  }

  function tell(text: string, isError = false) {
    message = text
    failed = isError
  }

  async function load() {
    try {
      opname = await api<Opname>(`/api/opnames/${id}`)
      items = opname.items ?? []
    } catch (err) {
      tell((err as Error).message, true)
    }
  }

  async function save(item: Item) {
    try {
      await put(`/api/opnames/${id}/items/${item.product_id}`, { physical_qty: item.physical_qty })
    } catch (err) {
      tell((err as Error).message, true)
    }
  }

  function typed(item: Item, e: Event) {
    const text = (e.currentTarget as HTMLInputElement).value
    const value = text === '' ? null : Math.max(0, Math.trunc(Number(text)))
    item.physical_qty = Number.isNaN(value) ? null : value
    save(item)
  }

  function scanned(e: KeyboardEvent) {
    if (e.key !== 'Enter') return
    e.preventDefault()
    const code = scan.trim()
    scan = ''
    if (!code) return
    const item = items.find((i) => i.barcode === code || i.barcode === code.toUpperCase())
    if (!item) {
      tell(`Barcode ${code} tidak ada di lembar hitung ini.`, true)
      return
    }
    item.physical_qty = (item.physical_qty ?? 0) + 1
    tell(`${item.name}: ${item.physical_qty}`)
    save(item)
  }

  async function postOpname() {
    if (!confirm(`Posting opname ini? Stok ${counted} produk yang dihitung akan disamakan dengan jumlah fisik.`)) return
    busy = true
    try {
      await post(`/api/opnames/${id}/post`)
      tell('Opname diposting. Stok sudah disesuaikan.')
      view = 'diff'
      await load()
    } catch (err) {
      tell((err as Error).message, true)
    } finally {
      busy = false
    }
  }

  async function remove() {
    if (!confirm('Hapus draft opname ini? Hitungan yang sudah diisi akan hilang.')) return
    await del(`/api/opnames/${id}`)
    location.hash = '#/opnames'
  }

  load()
</script>

<a class="back" href="#/opnames"><Icon name="back" size={16} /> Semua opname</a>

{#if opname}
  <div class="page-head">
    <div class="titles">
      <h1>Stok opname #{opname.id}</h1>
      <p>
        {opname.member_id ? opname.member_name : 'Barang toko'} · dibuat {dateTime(opname.created_at)}
        {#if opname.posted_at}· diposting {dateTime(opname.posted_at)}{/if}
        {#if opname.note}· {opname.note}{/if}
      </p>
    </div>
    {#if isDraft}
      <button onclick={remove}><Icon name="trash" /> Hapus draft</button>
      <button class="primary" disabled={busy || counted === 0} onclick={postOpname}>Posting</button>
    {:else}
      <span class="badge">Diposting</span>
    {/if}
  </div>

  <div class="summary">
    <div><b>{items.length}</b> produk</div>
    <div><b>{counted}</b> sudah dihitung</div>
    <div><b class:neg={diffs > 0}>{diffs}</b> selisih</div>
    {#if isDraft}
      <!-- svelte-ignore a11y_autofocus -->
      <input bind:value={scan} onkeydown={scanned} placeholder="Scan barcode: tiap scan menambah 1" autofocus />
    {/if}
  </div>

  {#if message}<p class="status" class:error={failed}>{message}</p>{/if}

  <div class="filters">
    <div class="search">
      <Icon name="search" />
      <input placeholder="Cari nama atau barcode" bind:value={search} />
    </div>
    <select bind:value={view}>
      <option value="all">Semua produk</option>
      <option value="uncounted">Belum dihitung</option>
      <option value="diff">Hanya yang selisih</option>
    </select>
  </div>

  <div class="table-card">
    <table>
      <thead>
        <tr>
          <th>Barcode</th>
          <th>Nama</th>
          <th class="num">Sistem</th>
          <th class="num">Fisik</th>
          <th class="num">Selisih</th>
        </tr>
      </thead>
      <tbody>
        {#each shown as item (item.product_id)}
          {@const d = diff(item)}
          <tr>
            <td class="code">{item.barcode || '—'}</td>
            <td class="strong">{item.name}</td>
            <td class="num">{item.system_qty}</td>
            <td class="num">
              {#if isDraft}
                <input
                  type="number"
                  min="0"
                  step="1"
                  value={item.physical_qty}
                  onchange={(e) => typed(item, e)}
                  placeholder="—"
                  aria-label="Jumlah fisik {item.name}"
                />
              {:else}
                {item.physical_qty ?? '—'}
              {/if}
            </td>
            <td class="num" class:neg={d < 0} class:pos={d > 0}>
              {item.physical_qty === null ? '—' : d > 0 ? `+${d}` : d}
            </td>
          </tr>
        {:else}
          <tr><td colspan="5" class="empty">Tidak ada produk untuk tampilan ini.</td></tr>
        {/each}
      </tbody>
    </table>
  </div>
{:else if message}
  <p class="error">{message}</p>
{/if}

<style>
  .summary {
    display: flex;
    align-items: center;
    gap: 1.6rem;
    margin-bottom: 0.9rem;
    padding: 0.8rem 1.1rem;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--card);
    color: var(--muted);
  }
  .summary b {
    margin-right: 0.25rem;
    font-family: var(--mono);
    font-size: 1.25rem;
    color: var(--text);
  }
  .summary b.neg {
    color: var(--danger);
  }
  .summary input {
    margin-left: auto;
    max-width: 320px;
  }
  .status {
    margin: 0 0 0.9rem;
    padding: 0.55rem 0.75rem;
    border-radius: 7px;
    background: var(--accent-wash);
    color: var(--accent-dark);
  }
  .status.error {
    background: var(--danger-wash);
    color: var(--danger);
  }
  td input {
    width: 5.5rem;
    height: 30px;
    font-family: var(--mono);
    text-align: right;
  }
</style>
