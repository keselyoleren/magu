<script lang="ts">
  import Icon from '../../components/Icon.svelte'
  import Modal from '../../components/Modal.svelte'
  import { api, post } from '../../lib/api'
  import { clearDraft, readDraft, writeDraft } from '../../lib/docDraft'
  import { focusFirst } from '../../lib/focus'
  import { dateTime, isoDate, rupiah } from '../../lib/format'
  import { listMembers, type Member } from '../../lib/members'
  import { printGoodsReceipt } from '../../lib/receipt'
  import type { Product } from '../products/Products.svelte'

  let { kind = 'receipts' }: { kind?: 'receipts' | 'returns' } = $props()

  const isReturn = $derived(kind === 'returns')
  const LIMIT = 1000
  const TYPE_QTY_KEY = 'receipt-type-qty'

  interface LastDoc {
    id: number
    received_at: string
    total_qty: number
    cancelled_at: string | null
  }

  function savedTypeQty() {
    try {
      return localStorage.getItem(TYPE_QTY_KEY) === '1'
    } catch {
      return false
    }
  }

  function rememberTypeQty() {
    try {
      localStorage.setItem(TYPE_QTY_KEY, typeQty ? '1' : '0')
    } catch {
      // Hanya kenyamanan: tanpa penyimpanan browser, pilihan ini kembali ke bawaan.
    }
  }

  let members = $state<Member[]>([])
  let memberId = $state<number | null>(null)
  let ownerBefore: number | null = null
  let products = $state<Product[]>([])
  let truncated = $state(false)
  // Jumlah per produk; kosong atau 0 berarti produk itu tidak ikut disimpan.
  let qty = $state<Record<number, number | null>>({})
  let filter = $state('')
  let view = $state<'all' | 'filled' | 'empty'>('all')
  let typeQty = $state(savedTypeQty())
  let note = $state('')
  let loading = $state(false)
  let error = $state('')
  let busy = $state(false)
  let filterInput = $state<HTMLInputElement>()
  let message = $state('')
  let otherOwner = $state<Product | null>(null)
  let draft = $state<{ name: string; barcode: string; price: number | null; qty: number | null } | null>(null)
  let saved = $state<{ id: number; total: number; lines: number; value: number; owner: string; notReady: number } | null>(null)
  let last = $state<LastDoc | null>(null)
  let printing = $state(false)
  // Baru menyimpan isian setelah isian lama dipulihkan, supaya tidak tertimpa daftar yang masih kosong.
  let ready = $state(false)

  const filled = $derived(products.filter((p) => (qty[p.id] ?? 0) > 0))
  const totalQty = $derived(filled.reduce((sum, p) => sum + (qty[p.id] ?? 0), 0))
  const totalValue = $derived(filled.reduce((sum, p) => sum + (qty[p.id] ?? 0) * p.price, 0))
  const notReady = $derived(filled.filter((p) => !p.is_active || p.price === 0).length)
  const ownerName = $derived(members.find((m) => m.id === memberId)?.name ?? 'Barang toko')
  const lastIsToday = $derived(!!last && isoDate(new Date(last.received_at)) === isoDate())
  const shown = $derived.by(() => {
    const q = filter.trim().toLowerCase()
    return products.filter(
      (p) =>
        (view === 'all' || (view === 'filled' ? (qty[p.id] ?? 0) > 0 : p.stock <= 0)) &&
        (!q || p.name.toLowerCase().includes(q) || p.barcode.toLowerCase().includes(q)),
    )
  })
  const draftReady = $derived(!!draft?.name.trim() && draft.price !== null && (draft.qty ?? 0) > 0)

  $effect(() => {
    if (ready && !saved) writeDraft(kind, { memberId, note, qty: Object.fromEntries(filled.map((p) => [p.id, qty[p.id] ?? 0])) })
  })

  // Ditampilkan supaya kiriman yang sama tidak tercatat dua kali oleh dua petugas.
  async function loadLast() {
    if (isReturn) return
    const owner = memberId
    last = null
    const docs = await api<LastDoc[]>(`/api/receipts?member_id=${owner ?? -1}`)
    if (owner === memberId) last = docs.find((d) => !d.cancelled_at) ?? null
  }

  async function loadProducts() {
    loading = true
    error = ''
    qty = {}
    filter = ''
    view = 'all'
    message = ''
    otherOwner = null
    draft = null
    ownerBefore = memberId
    loadLast()
    try {
      const params = new URLSearchParams({ q: '', member_id: String(memberId ?? -1), per_page: String(LIMIT) })
      const res = await api<{ items: Product[]; has_more: boolean }>(`/api/products?${params}`)
      products = res.items
      truncated = res.has_more
    } catch (err) {
      error = (err as Error).message
    } finally {
      loading = false
    }
  }

  // Satu dokumen hanya untuk satu pemilik, jadi berganti pemilik mengosongkan jumlah yang sudah diisi.
  function ownerChanged() {
    if (filled.length > 0 && !confirm(`Ganti pemilik? Jumlah ${filled.length} barang yang sudah diisi akan hilang.`)) {
      memberId = ownerBefore
      return
    }
    loadProducts()
  }

  function addOne(p: Product) {
    qty[p.id] = typeQty ? Math.max(qty[p.id] ?? 0, 1) : (qty[p.id] ?? 0) + 1
    filter = ''
    if (view === 'empty') view = 'all'
    message = `${p.name}: ${qty[p.id]} pcs`
    otherOwner = null
    requestAnimationFrame(() => {
      document.getElementById(`row-${p.id}`)?.scrollIntoView({ block: 'nearest' })
      if (!typeQty) return
      const box = document.getElementById(`qty-${p.id}`) as HTMLInputElement | null
      box?.focus()
      box?.select()
    })
  }

  function clearAll() {
    if (!confirm(`Kosongkan ${filled.length} barang yang sudah diisi?`)) return
    qty = {}
    note = ''
    message = ''
    view = 'all'
  }

  // Scanner mengetik barcode lalu Enter: barcode yang cocok persis langsung menambah 1 dan kolomnya dikosongkan,
  // jadi barang bisa di-scan beruntun. Tanpa Enter, kolom ini hanya menyaring daftar.
  async function filterKey(e: KeyboardEvent) {
    if (e.key !== 'Enter') return
    e.preventDefault()
    const text = filter.trim()
    if (!text) return
    const match = products.find((p) => p.barcode && (p.barcode === text || p.barcode === text.toUpperCase()))
    const hit = match ?? (shown.length === 1 ? shown[0] : undefined)
    if (hit) return addOne(hit)
    if (shown.length > 1) return

    // Tidak ada di daftar pemilik ini: bisa barang pemilik lain, di luar 1000 produk pertama, atau belum terdaftar.
    otherOwner = null
    message = ''
    try {
      const found = (await api<{ items: Product[] }>(`/api/products?q=${encodeURIComponent(text)}`)).items
      const exact = found.find((p) => p.barcode === text || p.barcode === text.toUpperCase())
      if (exact && exact.member_id === memberId) {
        products.push(exact)
        addOne(exact)
      } else if (exact) {
        otherOwner = exact
        filter = ''
      } else if (isReturn) {
        message = `"${text}" tidak ditemukan.`
      } else {
        startDraft(text)
      }
    } catch (err) {
      error = (err as Error).message
    }
  }

  async function switchOwner() {
    const p = otherOwner
    if (!p) return
    if (filled.length > 0 && !confirm(`Ganti pemilik? Jumlah ${filled.length} barang yang sudah diisi akan hilang.`)) return
    memberId = p.member_id
    await loadProducts()
    const loaded = products.find((x) => x.id === p.id)
    if (loaded) addOne(loaded)
    filterInput?.focus()
  }

  // Teks tanpa spasi yang memuat angka hampir pasti barcode dari scanner, selain itu dianggap nama yang diketik.
  function startDraft(text = '') {
    const isBarcode = /^\S*\d\S*$/.test(text)
    draft = { name: isBarcode ? '' : text, barcode: isBarcode ? text : '', price: null, qty: null }
    filter = ''
  }

  async function addDraft() {
    if (!draft || !draftReady) return
    busy = true
    error = ''
    try {
      const p = await post<Product>('/api/products', {
        barcode: draft.barcode,
        name: draft.name,
        price: draft.price,
        member_id: memberId,
        commission_bps: null,
        is_active: true,
      })
      products.unshift(p)
      qty[p.id] = draft.qty
      message = `${p.name} terdaftar dengan barcode ${p.barcode}.`
      draft = null
      filterInput?.focus()
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }

  // Enter di kotak jumlah kembali ke kolom scan; kalau tidak, form tersimpan sebelum semua barang diisi.
  function qtyKey(e: KeyboardEvent) {
    if (e.key !== 'Enter') return
    e.preventDefault()
    filterInput?.focus()
  }

  function draftKey(e: KeyboardEvent) {
    if (e.key !== 'Enter') return
    e.preventDefault()
    addDraft()
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      const { id } = await post<{ id: number }>(`/api/${kind}`, {
        member_id: memberId,
        note,
        items: filled.map((p) => ({ product_id: p.id, qty: qty[p.id] })),
      })
      clearDraft(kind)
      if (isReturn) location.hash = `#/${kind}`
      else saved = { id, total: totalQty, lines: filled.length, value: totalValue, owner: ownerName, notReady }
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }

  async function printSaved(id: number) {
    printing = true
    error = ''
    try {
      await printGoodsReceipt(id)
    } catch (err) {
      error = (err as Error).message
    } finally {
      printing = false
    }
  }

  function receiveMore() {
    saved = null
    note = ''
    loadProducts()
  }

  // Produk di luar daftar (lewat dari LIMIT) diambil satu per satu supaya jumlahnya tidak hilang diam-diam.
  async function restore(old: NonNullable<ReturnType<typeof readDraft>>) {
    const ids = Object.keys(old.qty).map(Number)
    const missing = ids.filter((id) => !products.some((p) => p.id === id))
    const extra = await Promise.all(missing.map((id) => api<Product>(`/api/products/${id}`).catch(() => null)))
    for (const p of extra) if (p && p.member_id === memberId) products.push(p)
    qty = old.qty
    note = old.note
    if (filled.length > 0) message = `Melanjutkan isian yang belum disimpan: ${filled.length} barang, ${totalQty} pcs.`
  }

  listMembers().then(async (m) => {
    members = m.filter((x) => x.is_active)
    const old = readDraft(kind)
    const usable = old && (old.memberId === null ? !isReturn : members.some((x) => x.id === old.memberId))
    memberId = usable ? old.memberId : isReturn ? (members[0]?.id ?? null) : null
    await loadProducts()
    if (usable) await restore(old)
    ready = true
  })
</script>

<Modal title={isReturn ? 'Retur barang ke penitip' : 'Terima barang'} wide onclose={() => (location.hash = `#/${kind}`)}>
  {#if saved}
    <div class="modal-form">
      <div class="form-body">
        <p class="success">
          <strong>{saved.total} pcs</strong> ({saved.lines} barang) dari {saved.owner} sudah masuk stok dengan nomor
          <strong>#{saved.id}</strong>. Nilai jualnya Rp {rupiah(saved.value)}.
        </p>
        {#if saved.notReady > 0}
          <p class="notice">
            <span>{saved.notReady} barang nonaktif atau belum punya harga, jadi belum siap dijual.</span>
            <a href="#/prices">Bereskan di Atur harga</a>
          </p>
        {/if}
        <p class="muted">Tanda terima untuk pegangan penitip. Label hanya perlu dicetak kalau barangnya belum berlabel.</p>
        {#if error}<p class="error">{error}</p>{/if}
      </div>
      <div class="form-actions">
        <button type="button" onclick={receiveMore}><Icon name="plus" /> Terima lagi</button>
        <a class="button" href="#/receipts">Selesai</a>
        <button type="button" disabled={printing} onclick={() => printSaved(saved!.id)}><Icon name="printer" /> Cetak tanda terima</button>
        <a class="button primary" href="#/labels/receipt/{saved.id}"><Icon name="tag" /> Cetak {saved.total} label</a>
      </div>
    </div>
  {:else}
    <form class="modal-form" onsubmit={save} use:focusFirst>
      <div class="form-body">
        <p class="modal-hint">
          {isReturn
            ? 'Pilih penitip, lalu isi jumlah di samping barang yang dikembalikan.'
            : 'Pilih pemilik, lalu scan barangnya atau isi jumlahnya. Barang yang belum terdaftar bisa langsung dibuat di sini.'}
          Isian tersimpan otomatis di komputer ini, jadi aman kalau jendela ini tertutup.
        </p>
        <div class="pair">
          <label>
            Pemilik barang
            <select bind:value={memberId} onchange={ownerChanged}>
              {#if !isReturn}<option value={null}>Barang toko</option>{/if}
              {#each members as m (m.id)}
                <option value={m.id}>{m.code} — {m.name}</option>
              {/each}
            </select>
          </label>
          <label>
            Cari atau scan
            <input bind:this={filterInput} bind:value={filter} onkeydown={filterKey} placeholder="Ketik nama untuk menyaring, atau scan barcode" />
          </label>
        </div>

        <div class="under-pair">
          <span class="last" class:warn={lastIsToday}>
            {#if last}
              Penerimaan terakhir {ownerName}: {dateTime(last.received_at)}, {rupiah(last.total_qty)} pcs.
              {#if lastIsToday}<strong>Pastikan ini bukan kiriman yang sama.</strong>{/if}
            {/if}
          </span>
          <label class="check"><input type="checkbox" bind:checked={typeQty} onchange={rememberTypeQty} /> Setelah scan, ketik jumlahnya</label>
        </div>

        {#if otherOwner}
          <p class="notice">
            <span><strong>{otherOwner.name}</strong> milik {otherOwner.member_name || 'toko'}, bukan pemilik yang dipilih.</span>
            {#if otherOwner.member_id !== null || !isReturn}
              <button type="button" class="link" onclick={switchOwner}>Ganti ke {otherOwner.member_name || 'barang toko'}</button>
            {/if}
          </p>
        {:else if message}
          <p class="scanned">{message}</p>
        {/if}

        {#if draft}
          <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
          <div class="draft" role="group" aria-label="Barang baru" onkeydown={draftKey} use:focusFirst>
            <p class="draft-title">Barang baru</p>
            <div class="pair">
              <label>Nama barang <input bind:value={draft.name} /></label>
              <label>Barcode <input bind:value={draft.barcode} placeholder="Kosongkan untuk dibuat otomatis" /></label>
            </div>
            <div class="pair">
              <label>Harga jual (Rp) <input type="number" bind:value={draft.price} min="0" step="1" /></label>
              <label>Jumlah datang <input type="number" bind:value={draft.qty} min="1" step="1" /></label>
            </div>
            <div class="draft-actions">
              <button type="button" onclick={() => (draft = null)}>Batal</button>
              <button type="button" disabled={busy || !draftReady} onclick={addDraft}>Tambahkan ke daftar</button>
            </div>
          </div>
        {/if}

        <div class="list-head">
          <span class="section">{filled.length} dari {products.length} barang · {rupiah(totalQty)} pcs · nilai jual Rp {rupiah(totalValue)}</span>
          <span class="tools">
            {#if filled.length > 0}<button type="button" class="link" onclick={clearAll}>Kosongkan</button>{/if}
            <select bind:value={view} aria-label="Tampilkan barang">
              <option value="all">Semua barang</option>
              <option value="filled">Yang diisi</option>
              <option value="empty">Stok habis</option>
            </select>
            {#if !isReturn && !draft}
              <button type="button" onclick={() => startDraft(filter.trim())}><Icon name="plus" /> Barang baru</button>
            {/if}
          </span>
        </div>

        <div class="list">
          {#each shown as p (p.id)}
            {@const n = qty[p.id] ?? 0}
            <label class="row" class:on={n > 0} id="row-{p.id}">
              <span class="name">
                <strong>{p.name}</strong>
                <small>
                  {p.barcode || 'tanpa barcode'} · Rp {rupiah(p.price)} · stok {p.stock}{#if n > 0}{' '}→ <b>{isReturn ? p.stock - n : p.stock + n}</b>{/if}
                </small>
              </span>
              <span class="flags">
                {#if !p.is_active}<span class="badge off">Nonaktif</span>{/if}
                {#if p.price === 0}<span class="badge off">Belum ada harga</span>{/if}
              </span>
              <input id="qty-{p.id}" type="number" min="0" step="1" bind:value={qty[p.id]} onkeydown={qtyKey} placeholder="0" aria-label="Jumlah {p.name}" />
            </label>
          {:else}
            <p class="muted empty">
              {#if loading}
                Memuat produk…
              {:else if products.length === 0}
                Pemilik ini belum punya produk. {isReturn ? '' : 'Pilih Barang baru untuk mendaftarkannya.'}
              {:else}
                Tidak ada produk yang cocok. {isReturn ? '' : 'Tekan Enter untuk mendaftarkannya sebagai barang baru.'}
              {/if}
            </p>
          {/each}
        </div>
        {#if truncated}
          <p class="muted">Hanya {LIMIT} produk pertama yang ditampilkan. Produk lain tetap bisa ditambah dengan scan barcodenya.</p>
        {/if}

        <label>Catatan <input bind:value={note} placeholder="Opsional, mis. nomor surat jalan" /></label>
        {#if !isReturn && notReady > 0}
          <p class="notice"><span>{notReady} barang nonaktif atau belum punya harga. Stoknya tetap masuk, tapi belum siap dijual.</span></p>
        {/if}
        {#if error}<p class="error">{error}</p>{/if}
      </div>
      <div class="form-actions">
        <a class="button" href="#/{kind}">Tutup</a>
        <button class="primary" disabled={busy || filled.length === 0}>
          {isReturn ? 'Simpan retur' : 'Simpan penerimaan'}{filled.length ? ` (${totalQty} pcs)` : ''}
        </button>
      </div>
    </form>
  {/if}
</Modal>

<style>
  .scanned {
    margin: 0;
    font-size: 0.9rem;
    color: var(--accent-dark);
  }
  .notice {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.8rem;
    margin: 0;
    padding: 0.5rem 0.75rem;
    border-radius: 7px;
    background: #f6ecd2;
    color: #7a5608;
  }
  .notice strong {
    font-weight: 500;
  }
  .notice a {
    color: inherit;
    font-weight: 500;
    white-space: nowrap;
  }
  .under-pair {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: 0.4rem 1rem;
    font-size: 0.86rem;
  }
  .under-pair .check {
    font-size: inherit;
    white-space: nowrap;
  }
  .last {
    color: var(--muted);
  }
  .last.warn {
    color: #7a5608;
  }
  .last strong {
    font-weight: 500;
  }
  .draft {
    display: grid;
    gap: 0.7rem;
    padding: 0.9rem 1rem;
    border: 1px solid var(--accent);
    border-radius: 8px;
    background: var(--accent-wash);
  }
  .draft-title {
    margin: 0;
    font-weight: 500;
  }
  .draft-actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
  }
  .list-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: 0.2rem;
    padding-top: 0.8rem;
    border-top: 1px dashed var(--line-strong);
  }
  .list-head .section {
    margin: 0;
    padding: 0;
    border: 0;
  }
  .tools {
    display: flex;
    align-items: center;
    gap: 0.9rem;
  }
  .tools select {
    width: auto;
    height: 32px;
  }
  .list {
    max-height: 42vh;
    min-height: 120px;
    overflow-y: auto;
    border: 1px solid var(--line);
    border-radius: 8px;
  }
  .row {
    display: grid;
    grid-template-columns: 1fr auto 6rem;
    align-items: center;
    gap: 0.8rem;
    padding: 0.5rem 0.8rem;
    border-bottom: 1px solid var(--line);
    color: var(--text);
    font-size: 1rem;
    font-weight: 400;
    cursor: text;
  }
  .row:last-child {
    border-bottom: 0;
  }
  .row.on {
    background: var(--accent-wash);
    box-shadow: inset 3px 0 0 var(--accent);
  }
  .name {
    display: grid;
    min-width: 0;
  }
  .name strong {
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  small {
    font-family: var(--mono);
    font-size: 0.78rem;
    color: var(--muted);
  }
  small b {
    font-weight: 500;
    color: var(--accent-dark);
  }
  .flags {
    display: flex;
    gap: 0.3rem;
  }
  .row input {
    height: 32px;
    font-family: var(--mono);
    text-align: right;
  }
  .empty {
    margin: 0;
    padding: 1.5rem;
    text-align: center;
  }
  p.muted {
    margin: 0;
  }
</style>
