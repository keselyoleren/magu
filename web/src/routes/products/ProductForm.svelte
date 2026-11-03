<script lang="ts">
  import Icon from '../../components/Icon.svelte'
  import Barcode from '../../components/Barcode.svelte'
  import Modal from '../../components/Modal.svelte'
  import { api, del, post, put } from '../../lib/api'
  import { focusFirst } from '../../lib/focus'
  import { bpsToPercent, percentToBps } from '../../lib/format'
  import { listMembers, type Member } from '../../lib/members'
  import type { Product } from './Products.svelte'

  let { id }: { id: number | null } = $props()

  let members = $state<Member[]>([])
  let barcode = $state('')
  let name = $state('')
  let price = $state<number | null>(null)
  let memberId = $state<number | null>(null)
  let commission = $state('')
  let isActive = $state(true)
  let stock = $state(0)
  let initialStock = $state<number | null>(null)
  let done = $state<{ name: string; barcode: string; qty: number; labels: string; warning: string } | null>(null)
  let ready = $state(false)
  let error = $state('')
  let busy = $state(false)
  let nameInput = $state<HTMLInputElement>()

  const owner = $derived(members.find((m) => m.id === memberId))

  async function load() {
    try {
      members = await listMembers()
      if (id !== null) {
        const p = await api<Product>(`/api/products/${id}`)
        barcode = p.barcode
        name = p.name
        price = p.price
        memberId = p.member_id
        commission = p.commission_bps === null ? '' : bpsToPercent(p.commission_bps)
        isActive = p.is_active
        stock = p.stock
      }
      ready = true
    } catch (err) {
      error = (err as Error).message
    }
  }

  // Scanner mengakhiri barcode dengan Enter; tanpa ini form langsung tersimpan sebelum nama dan harga diisi.
  function barcodeKey(e: KeyboardEvent) {
    if (e.key !== 'Enter') return
    e.preventDefault()
    nameInput?.focus()
  }

  async function makeBarcode() {
    try {
      barcode = (await post<{ barcode: string }>('/api/products/barcode', { member_id: memberId })).barcode
      nameInput?.focus()
    } catch (err) {
      error = (err as Error).message
    }
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    let bps: number | null = null
    if (memberId !== null && commission.trim() !== '') {
      bps = percentToBps(commission)
      if (bps === null) {
        error = 'Komisi harus antara 0 dan 100%'
        return
      }
    }
    busy = true
    error = ''
    try {
      const data = { barcode, name, price, member_id: memberId, commission_bps: bps, is_active: isActive }
      if (id !== null) {
        await put(`/api/products/${id}`, data)
        location.hash = '#/products'
        return
      }
      const created = await post<Product>('/api/products', data)
      done = await receiveInitialStock(created)
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }

  // Stok awal masuk lewat penerimaan supaya tercatat di stok titipan penitip, sama seperti barang datang lainnya.
  // Produknya sudah tersimpan di titik ini, jadi kegagalan di sini tidak boleh membuat admin menyimpan form dua kali.
  async function receiveInitialStock(p: Product) {
    const qty = initialStock ?? 0
    const result = { name: p.name, barcode: p.barcode, qty, labels: `#/labels/product/${p.id}`, warning: '' }
    if (qty <= 0) return result
    try {
      const receipt = await post<{ id: number }>('/api/receipts', {
        member_id: memberId,
        note: 'Stok awal',
        items: [{ product_id: p.id, qty }],
      })
      result.labels = `#/labels/receipt/${receipt.id}`
    } catch (err) {
      result.qty = 0
      result.warning = `Stok awal belum masuk: ${(err as Error).message}. Tambahkan lewat Terima barang.`
    }
    return result
  }

  function addAnother() {
    done = null
    barcode = ''
    name = ''
    price = null
    commission = ''
    initialStock = null
  }

  async function remove() {
    if (!confirm(`Hapus produk "${name}"? Tindakan ini tidak bisa dibatalkan.`)) return
    busy = true
    error = ''
    try {
      await del(`/api/products/${id}`)
      location.hash = '#/products'
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }

  load()
</script>

<Modal title={id === null ? 'Produk baru' : 'Ubah produk'} onclose={() => (location.hash = '#/products')}>
  {#if done}
    <div class="modal-form">
      <div class="form-body">
        <p class="success">
          <strong>{done.name}</strong> tersimpan{done.qty > 0 ? `, stok ${done.qty}` : ''}. Tinggal tempel labelnya.
        </p>
        {#if done.warning}<p class="error">{done.warning}</p>{/if}
        <div class="barcode-preview">
          <div class="bars"><Barcode value={done.barcode} /></div>
          <code>{done.barcode}</code>
        </div>
      </div>
      <div class="form-actions">
        <button type="button" onclick={addAnother}><Icon name="plus" /> Tambah barang lain</button>
        <a class="button" href="#/products">Selesai</a>
        <a class="button primary" href={done.labels}><Icon name="tag" /> Cetak label</a>
      </div>
    </div>
  {:else if ready}
    <form class="modal-form" onsubmit={save} use:focusFirst>
      <div class="form-body">
        <p class="modal-hint">
          {id === null
            ? 'Scan barcode bawaan barang, atau kosongkan dan sistem yang membuatkan. Label bisa langsung dicetak setelah disimpan.'
            : 'Barcode yang dikosongkan dibuat otomatis: kode pemilik + nomor urut.'}
        </p>
        <label>
          Barcode
          <span class="with-button">
            <input bind:value={barcode} onkeydown={barcodeKey} placeholder="Scan, ketik, atau kosongkan untuk dibuat otomatis" />
            <button type="button" onclick={makeBarcode} title="Untuk barang yang belum punya barcode">Buat barcode</button>
          </span>
        </label>
        {#if barcode.trim()}
          <div class="barcode-preview">
            <div class="bars"><Barcode value={barcode.trim()} /></div>
            <code>{barcode.trim()}</code>
          </div>
        {/if}
        <label>Nama <input bind:this={nameInput} bind:value={name} required /></label>
        <div class="pair">
          <label>Harga (Rp) <input type="number" bind:value={price} min="0" step="1" required /></label>
          {#if id === null}
            <label>
              Stok awal
              <input type="number" bind:value={initialStock} min="0" step="1" placeholder="Jumlah barang yang ada sekarang" />
            </label>
          {:else}
            <div class="stock-field">
              Stok
              <span><b>{stock}</b><a class="row-link" href="#/products/{id}/stock">Ubah stok</a></span>
            </div>
          {/if}
        </div>
        <p class="section">Kepemilikan</p>
        <div class="pair">
          <label>
            Pemilik
            <select bind:value={memberId}>
              <option value={null}>Milik toko</option>
              {#each members as m (m.id)}
                <option value={m.id}>{m.code} — {m.name}</option>
              {/each}
            </select>
          </label>
          {#if owner}
            <label>
              Komisi toko (%)
              <input
                bind:value={commission}
                inputmode="decimal"
                placeholder="Ikut member: {bpsToPercent(owner.commission_bps)}%"
              />
            </label>
          {/if}
        </div>
        <label class="check"><input type="checkbox" bind:checked={isActive} /> Aktif</label>
        {#if error}<p class="error">{error}</p>{/if}
      </div>
      <div class="form-actions">
        {#if id !== null}
          <button type="button" class="delete" disabled={busy} onclick={remove}><Icon name="trash" /> Hapus</button>
        {/if}
        <a class="button" href="#/products">Batal</a>
        <button class="primary" disabled={busy}>Simpan</button>
      </div>
    </form>
  {:else if error}
    <p class="error">{error}</p>
  {/if}
</Modal>

<style>
  .stock-field {
    display: grid;
    gap: 0.3rem;
    font-size: 0.82rem;
    font-weight: 500;
    color: var(--muted);
  }
  .stock-field span {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 36px;
  }
  .stock-field b {
    font-family: var(--mono);
    font-size: 1.1rem;
    font-weight: 500;
    color: var(--text);
  }
  .delete {
    margin-right: auto;
    color: var(--danger);
  }
  .barcode-preview {
    display: grid;
    justify-items: center;
    gap: 0.3rem;
    padding: 0.8rem 1rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: #fff;
  }
  .bars {
    width: min(100%, 340px);
    height: 64px;
  }
  code {
    font-family: var(--mono);
    font-size: 0.85rem;
    letter-spacing: 0.08em;
  }
</style>
