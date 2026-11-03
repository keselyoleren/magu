<script lang="ts">
  import Modal from '../../components/Modal.svelte'
  import { api, post } from '../../lib/api'
  import { focusFirst } from '../../lib/focus'
  import { rupiah } from '../../lib/format'
  import type { Product } from './Products.svelte'

  let { id }: { id: number } = $props()

  let product = $state<Product | null>(null)
  let stock = $state<number | null>(null)
  let note = $state('')
  let error = $state('')
  let busy = $state(false)

  const change = $derived(product && stock !== null ? stock - product.stock : 0)

  async function load() {
    try {
      product = await api<Product>(`/api/products/${id}`)
    } catch (err) {
      error = (err as Error).message
    }
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      await post('/api/products/stock', { note, items: [{ id, stock }] })
      location.hash = '#/products'
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }

  load()
</script>

<Modal title="Ubah stok" onclose={() => (location.hash = '#/products')}>
  {#if product}
    <form class="modal-form" onsubmit={save} use:focusFirst>
      <div class="form-body">
        <p class="modal-hint">
          Isi jumlah yang benar-benar ada di rak. Kalau ada kiriman barang, pakai
          <a href="#/receipts/new">Terima barang</a> supaya tercatat sebagai barang masuk.
        </p>
        <div class="current">
          <strong>{product.name}</strong>
          <span>Stok di sistem <b>{rupiah(product.stock)}</b></span>
        </div>
        <label>
          Stok yang benar
          <input type="number" bind:value={stock} min="0" step="1" required placeholder={String(Math.max(product.stock, 0))} />
        </label>
        {#if change !== 0}
          <p class="change" class:neg={change < 0} class:pos={change > 0}>
            Stok {change > 0 ? 'bertambah' : 'berkurang'} {rupiah(Math.abs(change))}
          </p>
        {/if}
        <label>Alasan <input bind:value={note} placeholder="Opsional, mis. 2 rusak, salah hitung" /></label>
        {#if error}<p class="error">{error}</p>{/if}
      </div>
      <div class="form-actions">
        <a class="button" href="#/products">Batal</a>
        <button class="primary" disabled={busy || stock === null || change === 0}>Simpan stok</button>
      </div>
    </form>
  {:else if error}
    <p class="error">{error}</p>
  {/if}
</Modal>

<style>
  .current {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.7rem 0.9rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: #faf7ef;
  }
  .current strong {
    font-weight: 500;
  }
  .current span {
    color: var(--muted);
    white-space: nowrap;
  }
  .current b {
    margin-left: 0.25rem;
    font-family: var(--mono);
    font-size: 1.2rem;
    font-weight: 500;
    color: var(--text);
  }
  .change {
    margin: 0;
    font-weight: 500;
  }
</style>
