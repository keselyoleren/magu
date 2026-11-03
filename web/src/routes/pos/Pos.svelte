<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '../../components/Icon.svelte'
  import QrCode from '../../components/QrCode.svelte'
  import { api, post } from '../../lib/api'
  import { catalog, findByBarcode, searchCatalog, type CatalogItem } from '../../lib/catalog.svelte'
  import { connectEvents } from '../../lib/events'
  import { rupiah } from '../../lib/format'
  import { printReceipt } from '../../lib/receipt'
  import { loadSettings, type Transaction } from '../../lib/sales'
  import { onScan } from '../../lib/scanner'
  import { uuid } from '../../lib/uuid'

  interface Line extends CatalogItem {
    qty: number
  }

  type Mode = 'cash' | 'debit' | 'split' | 'qris'

  let cart = $state<Line[]>([])
  let clientRef = $state(uuid())
  let mode = $state<Mode>('cash')
  let tendered = $state<number | null>(null)
  let debitAmount = $state<number | null>(null)
  let reference = $state('')
  let query = $state('')
  let lastAdded = $state(0)
  let notice = $state('')
  let error = $state('')
  let busy = $state(false)
  // sale = transaksi yang sudah tersimpan di server: menunggu pembayaran (pending) atau lunas (paid).
  let sale = $state<Transaction | null>(null)
  let switching = $state(false)
  let printNote = $state('')
  let qrisEnabled = $state(false)
  let searchInput = $state<HTMLInputElement>()

  const pieces = $derived(cart.reduce((sum, l) => sum + l.qty, 0))
  const total = $derived(sale ? sale.total : cart.reduce((sum, l) => sum + l.price * l.qty, 0))
  const results = $derived(searchCatalog(query))

  const paidPayments = $derived((sale?.payments ?? []).filter((p) => p.status === 'paid'))
  const isPaid = $derived(sale?.status === 'paid')
  const qr = $derived(
    sale?.status === 'pending' && !switching
      ? sale.payments?.find((p) => p.method === 'qris' && p.status === 'pending' && p.qr_string)
      : undefined,
  )
  const due = $derived(total - paidPayments.reduce((sum, p) => sum + p.amount, 0))
  const paidChange = $derived(paidPayments.reduce((sum, p) => sum + ((p.tendered ?? p.amount) - p.amount), 0))

  const debitPart = $derived(mode === 'debit' ? due : mode === 'split' ? (debitAmount ?? 0) : 0)
  const cashDue = $derived(mode === 'qris' ? 0 : due - debitPart)
  const change = $derived((tendered ?? cashDue) - cashDue)
  const quickCash = $derived(
    [...new Set([cashDue, ...[5000, 10000, 20000, 50000, 100000].map((d) => Math.ceil(cashDue / d) * d)])].slice(0, 5),
  )
  const problem = $derived.by(() => {
    if (cart.length === 0) return 'Keranjang kosong'
    if (mode === 'split' && (debitPart <= 0 || debitPart >= due)) return 'Isi nominal debit, kurang dari total'
    if (cashDue > 0 && change < 0) return 'Uang diterima kurang'
    return ''
  })

  function add(item: CatalogItem) {
    if (isPaid) newSale()
    if (sale) {
      notice = 'Selesaikan atau batalkan pembayaran yang sedang menunggu dulu.'
      return
    }
    const line = cart.find((l) => l.id === item.id)
    if (line) line.qty += 1
    else cart.push({ ...item, qty: 1 })
    lastAdded = item.id
    query = ''
    notice = ''
    error = ''
  }

  function scanned(barcode: string) {
    const item = findByBarcode(barcode)
    if (item) add(item)
    else notice = `Barcode ${barcode} tidak dikenal.`
  }

  function searchKey(e: KeyboardEvent) {
    if (e.key === 'Escape') query = ''
    if (e.key !== 'Enter') return
    e.preventDefault()
    const item = findByBarcode(query.trim()) ?? results[0]
    if (item) add(item)
    else if (query.trim()) notice = `"${query.trim()}" tidak ditemukan.`
  }

  function setQty(line: Line, qty: number) {
    if (qty <= 0) cart.splice(cart.indexOf(line), 1)
    else line.qty = Math.trunc(qty)
  }

  async function pay() {
    if (problem || busy || isPaid || qr) return
    busy = true
    error = ''
    const payments = []
    if (mode === 'qris') payments.push({ method: 'qris', amount: due })
    if (debitPart > 0) payments.push({ method: 'debit', amount: debitPart, reference: reference.trim() || null })
    if (cashDue > 0) payments.push({ method: 'cash', amount: cashDue, tendered: tendered ?? cashDue })
    try {
      const result = sale
        ? await post<Transaction>(`/api/transactions/${sale.id}/payments`, { payments })
        : await post<Transaction>('/api/checkout', {
            client_ref: clientRef,
            lines: cart.map((l) => ({ product_id: l.id, qty: l.qty })),
            payments,
          })
      received(result)
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }

  // Semua kabar dari server (balasan bayar, hasil refresh) lewat sini, jadi struk hanya tercetak
  // tepat saat transaksi berpindah dari belum lunas ke lunas.
  function received(next: Transaction) {
    const wasPaid = isPaid
    const hadQr = !!qr
    sale = next
    if (next.status === 'paid') {
      switching = false
      if (!wasPaid) print(true)
      return
    }
    const pendingQr = next.payments?.some((p) => p.method === 'qris' && p.status === 'pending' && p.qr_string)
    if (!pendingQr) {
      switching = false
      mode = 'cash'
      error = hadQr
        ? 'QR kedaluwarsa atau dibatalkan. Pilih cara bayar lain.'
        : 'QRIS tidak bisa dibuat. Pilih cara bayar lain.'
    }
  }

  // Event WebSocket hanya pemicu; keadaan yang ditampilkan selalu diambil ulang dari server.
  async function refresh() {
    if (!sale || sale.status !== 'pending') return
    const next = await api<Transaction>(`/api/transactions/${sale.id}`)
    if (sale?.id === next.id) received(next)
  }

  async function checkStatus() {
    if (!qr) return
    busy = true
    error = ''
    try {
      const { changed } = await post<{ changed: boolean }>(`/api/payments/${qr.id}/check`)
      if (changed) await refresh()
      else notice = 'Belum ada pembayaran masuk.'
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }

  function switchMethod() {
    switching = true
    mode = 'cash'
    error = ''
  }

  async function cancelSale() {
    if (!sale || !confirm('Batalkan transaksi ini? Uang yang sudah diterima dikembalikan manual.')) return
    try {
      await post(`/api/transactions/${sale.id}/void`)
      newSale()
    } catch (err) {
      error = (err as Error).message
    }
  }

  // Transaksi sudah tersimpan sebelum cetak; kegagalan printer hanya ditampilkan, tidak membatalkan apa pun.
  async function print(openDrawer: boolean) {
    if (!sale) return
    printNote = 'Mencetak…'
    try {
      await printReceipt(sale, openDrawer)
      printNote = 'Struk dicetak.'
    } catch (err) {
      printNote = `Struk tidak tercetak: ${(err as Error).message}`
    }
  }

  function newSale() {
    cart = []
    clientRef = uuid()
    mode = 'cash'
    tendered = null
    debitAmount = null
    reference = ''
    sale = null
    switching = false
    printNote = ''
    error = ''
    notice = ''
    searchInput?.focus()
  }

  function shortcut(e: KeyboardEvent) {
    if (e.key === 'F2') {
      e.preventDefault()
      searchInput?.focus()
    } else if (e.key === 'F9') {
      e.preventDefault()
      pay()
    } else if (isPaid && e.key === 'Enter') {
      e.preventDefault()
      newSale()
    }
  }

  onMount(() => {
    loadSettings().then((s) => (qrisEnabled = s.qris_enabled))
    const stopScan = onScan(scanned)
    const stopEvents = connectEvents((e) => {
      const data = e.data as { transaction_id: number }
      if (e.event === 'payment.updated' && data.transaction_id === sale?.id) refresh()
    }, refresh)
    return () => {
      stopScan()
      stopEvents()
    }
  })

  // Kolom cari nonaktif selama katalog dimuat, jadi fokusnya baru bisa diberikan setelah siap.
  $effect(() => {
    if (catalog.ready) searchInput?.focus()
  })
</script>

<svelte:window onkeydown={shortcut} />

<div class="register">
  <section class="cart">
    <div class="search">
      <Icon name="search" size={20} />
      <input
        bind:this={searchInput}
        bind:value={query}
        onkeydown={searchKey}
        placeholder={catalog.ready ? 'Scan barcode, atau ketik nama barang (F2)' : 'Memuat katalog…'}
        disabled={!catalog.ready}
      />
      {#if results.length > 0}
        <div class="results">
          {#each results as item, i (item.id)}
            <button class:first={i === 0} onclick={() => add(item)}>
              <span>{item.name}</span>
              <small>{item.barcode}</small>
              <b>{rupiah(item.price)}</b>
            </button>
          {/each}
        </div>
      {/if}
    </div>

    {#if notice}<p class="notice">{notice}</p>{/if}

    <div class="lines" inert={!!sale}>
      {#each cart as line (line.id)}
        <div class="line" class:fresh={line.id === lastAdded}>
          <div class="name">
            <strong>{line.name}</strong>
            <small>{rupiah(line.price)}</small>
          </div>
          <div class="qty">
            <button onclick={() => setQty(line, line.qty - 1)} aria-label="Kurangi {line.name}"><Icon name="minus" size={16} /></button>
            <input
              type="number"
              min="1"
              value={line.qty}
              onchange={(e) => setQty(line, Number(e.currentTarget.value))}
              aria-label="Jumlah {line.name}"
            />
            <button onclick={() => setQty(line, line.qty + 1)} aria-label="Tambah {line.name}"><Icon name="plus" size={16} /></button>
          </div>
          <span class="sub">{rupiah(line.price * line.qty)}</span>
          <button class="remove" onclick={() => setQty(line, 0)} aria-label="Hapus {line.name}"><Icon name="close" size={16} /></button>
        </div>
      {:else}
        <div class="empty">
          <Icon name="register" size={40} />
          <p>Scan barang untuk memulai transaksi</p>
        </div>
      {/each}
    </div>
  </section>

  <aside>
    {#if isPaid && sale}
      <div class="done">
        <span class="tag">Lunas · {sale.invoice_no}</span>
        <p>Kembalian</p>
        <b>{rupiah(paidChange)}</b>
        <small>Total {rupiah(sale.total)}</small>
        {#if printNote}<p class="print">{printNote}</p>{/if}
      </div>
      <div class="actions">
        <button onclick={() => print(false)}><Icon name="printer" /> Cetak ulang</button>
        <button class="pay" onclick={newSale}>Transaksi baru <kbd>Enter</kbd></button>
      </div>
    {:else if qr && sale}
      <div class="waiting">
        <span class="tag">Menunggu QRIS · {sale.invoice_no}</span>
        <div class="qr"><QrCode value={qr.qr_string} /></div>
        <b>{rupiah(qr.amount)}</b>
        <p>Minta pembeli memindai kode ini. Layar berubah sendiri saat pembayaran masuk.</p>
      </div>
      {#if error}<p class="error">{error}</p>{/if}
      <div class="actions three">
        <button onclick={cancelSale}>Batalkan</button>
        <button onclick={switchMethod}>Ganti cara bayar</button>
        <button class="pay" onclick={checkStatus} disabled={busy}>Cek status</button>
      </div>
    {:else}
      <div class="total">
        <span>{cart.length} barang · {pieces} pcs</span>
        <p>{sale ? 'Sisa yang harus dibayar' : 'Total'}</p>
        <b>{rupiah(due)}</b>
      </div>

      <div class="modes" class:four={qrisEnabled}>
        <button class:on={mode === 'cash'} onclick={() => (mode = 'cash')}>Tunai</button>
        <button class:on={mode === 'debit'} onclick={() => (mode = 'debit')}>Debit</button>
        <button class:on={mode === 'split'} onclick={() => (mode = 'split')}>Split</button>
        {#if qrisEnabled}
          <button class:on={mode === 'qris'} onclick={() => (mode = 'qris')}>QRIS</button>
        {/if}
      </div>

      <div class="fields">
        {#if mode === 'qris'}
          <p class="hint">Kode QR untuk {rupiah(due)} ditampilkan setelah menekan Bayar.</p>
        {/if}
        {#if mode === 'split'}
          <label>Nominal debit <input type="number" min="1" bind:value={debitAmount} placeholder="0" /></label>
        {/if}
        {#if mode === 'debit' || mode === 'split'}
          <label>Kode approval EDC <input bind:value={reference} placeholder="Opsional" /></label>
        {/if}
        {#if mode === 'cash' || mode === 'split'}
          <label>
            Uang diterima{mode === 'split' ? ` (tunai ${rupiah(Math.max(cashDue, 0))})` : ''}
            <input type="number" min="0" bind:value={tendered} placeholder="Uang pas" />
          </label>
          <div class="quick">
            {#each quickCash as amount}
              <button onclick={() => (tendered = amount)}>{amount === cashDue ? 'Uang pas' : rupiah(amount)}</button>
            {/each}
          </div>
          <div class="change" class:short={change < 0}>
            <span>{change < 0 ? 'Kurang' : 'Kembali'}</span>
            <b>{rupiah(Math.abs(change))}</b>
          </div>
        {/if}
      </div>

      {#if error}<p class="error">{error}</p>{/if}

      <div class="actions">
        {#if sale}
          <button onclick={cancelSale}>Batalkan</button>
        {:else}
          <button onclick={newSale} disabled={cart.length === 0}>Kosongkan</button>
        {/if}
        <button class="pay" onclick={pay} disabled={!!problem || busy} title={problem}>
          {busy ? 'Menyimpan…' : 'Bayar'} <kbd>F9</kbd>
        </button>
      </div>
    {/if}
  </aside>
</div>

<style>
  .register {
    display: grid;
    grid-template-columns: 1fr 380px;
    height: 100%;
    min-height: 0;
  }

  .cart {
    display: flex;
    flex-direction: column;
    min-height: 0;
    padding: 1.2rem 1.4rem;
  }

  .search {
    position: relative;
    color: var(--muted);
  }
  .search > :global(svg) {
    position: absolute;
    left: 0.9rem;
    top: 14px;
  }
  .search input {
    height: 48px;
    padding-left: 2.8rem;
    font-size: 1.1rem;
    border-radius: 10px;
  }

  .results {
    position: absolute;
    z-index: 5;
    top: 54px;
    left: 0;
    right: 0;
    display: grid;
    background: var(--card);
    border: 1px solid var(--line-strong);
    border-radius: 10px;
    box-shadow: 0 14px 30px rgba(15, 31, 26, 0.14);
    overflow: hidden;
  }
  .results button {
    display: grid;
    grid-template-columns: 1fr auto 6rem;
    gap: 1rem;
    justify-items: start;
    height: auto;
    padding: 0.65rem 0.9rem;
    border: 0;
    border-bottom: 1px solid var(--line);
    border-radius: 0;
    font-weight: 400;
  }
  .results button.first,
  .results button:hover {
    background: var(--accent-wash);
  }
  .results small,
  .results b {
    font-family: var(--mono);
  }
  .results small {
    color: var(--muted);
  }
  .results b {
    justify-self: end;
    font-weight: 500;
  }

  .notice {
    margin: 0.7rem 0 0;
    padding: 0.55rem 0.8rem;
    border-radius: 7px;
    background: #f6ecd2;
    color: #7a5608;
  }

  .lines {
    flex: 1;
    min-height: 0;
    margin-top: 0.9rem;
    overflow-y: auto;
    background: var(--card);
    border: 1px solid var(--line);
    border-radius: 10px;
  }

  .line {
    display: grid;
    grid-template-columns: 1fr auto 7.5rem auto;
    align-items: center;
    gap: 1rem;
    padding: 0.7rem 1rem;
    border-bottom: 1px solid var(--line);
  }
  .line.fresh {
    background: #faf7ef;
    box-shadow: inset 3px 0 0 var(--amber);
  }
  .name {
    display: grid;
    min-width: 0;
  }
  .name strong {
    font-size: 1.05rem;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .name small,
  .sub {
    font-family: var(--mono);
  }
  .name small {
    color: var(--muted);
  }
  .sub {
    text-align: right;
    font-size: 1.1rem;
  }

  .qty {
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }
  .qty button {
    width: 34px;
    height: 34px;
    padding: 0;
  }
  .qty input {
    width: 3.6rem;
    height: 34px;
    padding: 0 0.3rem;
    font-family: var(--mono);
    font-size: 1.05rem;
    text-align: center;
  }
  .remove {
    width: 30px;
    height: 30px;
    padding: 0;
    border-color: transparent;
    background: none;
    color: var(--muted);
  }

  .empty {
    display: grid;
    place-items: center;
    align-content: center;
    gap: 0.6rem;
    height: 100%;
    color: #a3a79f;
  }
  .empty p {
    margin: 0;
  }

  aside {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    padding: 1.2rem;
    background: var(--card);
    border-left: 1px solid var(--line);
  }

  .total,
  .done,
  .waiting {
    padding: 1.1rem 1.2rem 1.2rem;
    border-radius: 10px;
    color: #c9d6cf;
    background:
      repeating-linear-gradient(90deg, transparent 0 61px, rgba(255, 255, 255, 0.03) 61px 62px),
      linear-gradient(180deg, var(--ink-soft), var(--ink));
  }
  .total span,
  .done small,
  .tag {
    font-family: var(--mono);
    font-size: 0.75rem;
    letter-spacing: 0.04em;
  }
  .total p,
  .done p {
    margin: 0.9rem 0 0;
    font-size: 0.85rem;
  }
  .total b,
  .done b {
    display: block;
    font-family: var(--mono);
    font-size: 2.6rem;
    font-weight: 500;
    line-height: 1.1;
    color: #fff;
  }
  .done {
    flex: 1;
  }
  .tag {
    display: inline-block;
    padding: 0.2rem 0.6rem;
    border-radius: 999px;
    background: var(--amber);
    color: var(--ink);
    font-weight: 500;
  }
  .done .print {
    margin-top: 1.4rem;
    color: #fff;
  }

  .waiting {
    flex: 1;
    display: grid;
    align-content: start;
    justify-items: center;
    gap: 0.8rem;
    text-align: center;
  }
  .waiting .tag {
    justify-self: start;
  }
  .qr {
    width: min(100%, 260px);
    padding: 0.5rem;
    border-radius: 12px;
    background: #fff;
  }
  .waiting b {
    font-family: var(--mono);
    font-size: 2rem;
    font-weight: 500;
    color: #fff;
  }
  .waiting p {
    margin: 0;
    font-size: 0.85rem;
  }
  .hint {
    margin: 0;
    color: var(--muted);
  }
  .actions.three {
    grid-template-columns: auto auto 1fr;
  }
  .modes.four {
    grid-template-columns: repeat(4, 1fr);
  }

  .modes {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 0.4rem;
  }
  .modes button {
    height: 42px;
  }
  .modes button.on {
    background: var(--ink);
    border-color: var(--ink);
    color: #fff;
  }

  .fields {
    flex: 1;
    display: grid;
    align-content: start;
    gap: 0.8rem;
  }
  .fields input {
    height: 44px;
    font-family: var(--mono);
    font-size: 1.15rem;
  }
  .quick {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }
  .quick button {
    height: 32px;
    padding: 0 0.7rem;
    font-family: var(--mono);
    font-size: 0.85rem;
    font-weight: 400;
  }
  .change {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    padding: 0.7rem 0.9rem;
    border-radius: 8px;
    background: var(--accent-wash);
    color: var(--accent-dark);
  }
  .change.short {
    background: var(--danger-wash);
    color: var(--danger);
  }
  .change b {
    font-family: var(--mono);
    font-size: 1.5rem;
    font-weight: 500;
  }

  .actions {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 0.6rem;
  }
  .actions button {
    height: 54px;
  }
  .pay {
    background: var(--accent);
    border-color: var(--accent);
    color: #fff;
    font-size: 1.15rem;
  }
  .pay:hover:not(:disabled) {
    background: var(--accent-dark);
    border-color: var(--accent-dark);
  }
  kbd {
    padding: 0.1rem 0.4rem;
    border: 1px solid rgba(255, 255, 255, 0.35);
    border-radius: 4px;
    font-family: var(--mono);
    font-size: 0.7rem;
  }

  @media (max-width: 900px) {
    .register {
      grid-template-columns: 1fr;
      height: auto;
    }
    aside {
      border-left: 0;
      border-top: 1px solid var(--line);
    }
  }
</style>
