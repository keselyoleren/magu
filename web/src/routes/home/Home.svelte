<script lang="ts">
  import Icon, { type IconName } from '../../components/Icon.svelte'
  import { api } from '../../lib/api'
  import { rupiah } from '../../lib/format'
  import type { Recap } from '../../lib/sales'
  import { session } from '../../lib/session.svelte'

  interface Dashboard {
    no_price: number
    inactive_with_stock: number
    out_of_stock: number
    no_barcode: number
    pending_transactions: number
    outstanding_payout: number
    unpaid_settlements: number
    draft_opnames: number
    week: { day: string; gross: number }[]
    low_stock: { name: string; count: number }[]
    top_products: { name: string; count: number }[]
  }

  const tasks: { icon: IconName; title: string; text: string; href: string }[] = [
    { icon: 'inbox', title: 'Barang datang', text: 'Tambah stok dari kiriman penitip atau belanja toko', href: '#/receipts/new' },
    { icon: 'plus', title: 'Barang baru', text: 'Daftarkan barang yang belum ada, sekalian stok awalnya', href: '#/products/new' },
    { icon: 'sliders', title: 'Betulkan stok', text: 'Samakan stok dengan isi rak, satu atau banyak barang sekaligus', href: '#/stocks' },
    { icon: 'tag', title: 'Cetak label', text: 'Pilih barang dan jumlah label yang mau ditempel', href: '#/labels' },
  ]

  const weekday = new Intl.DateTimeFormat('id-ID', { weekday: 'short' })
  const longDate = new Intl.DateTimeFormat('id-ID', { dateStyle: 'full' })

  let recap = $state<Recap | null>(null)
  let data = $state<Dashboard | null>(null)

  const peak = $derived(Math.max(1, ...(data?.week ?? []).map((d) => d.gross)))
  const weekTotal = $derived((data?.week ?? []).reduce((sum, d) => sum + d.gross, 0))

  // Hanya hal yang benar-benar butuh tindakan yang ditampilkan; daftar kosong berarti semuanya beres.
  const todo = $derived.by(() => {
    if (!data) return []
    const items: { icon: IconName; text: string; action: string; href: string }[] = []
    const add = (show: boolean, icon: IconName, text: string, action: string, href: string) =>
      show && items.push({ icon, text, action, href })
    add(data.no_price > 0, 'edit', `${data.no_price} produk belum punya harga`, 'Atur harga', '#/prices')
    add(data.inactive_with_stock > 0, 'box', `${data.inactive_with_stock} produk berstok tapi nonaktif, belum bisa dijual`, 'Aktifkan', '#/prices')
    add(data.pending_transactions > 0, 'clock', `${data.pending_transactions} transaksi menunggu pembayaran`, 'Lihat', '#/transactions')
    add(data.outstanding_payout > 0, 'wallet', `Rp ${rupiah(data.outstanding_payout)} hak penitip belum dibagi`, 'Bagi hasil', '#/settlements')
    add(data.unpaid_settlements > 0, 'wallet', `Rp ${rupiah(data.unpaid_settlements)} sudah dibagi tapi belum dibayar`, 'Lihat', '#/settlements')
    add(data.out_of_stock > 0, 'inbox', `${data.out_of_stock} produk aktif stoknya habis`, 'Terima barang', '#/receipts/new')
    add(data.draft_opnames > 0, 'clipboard', `${data.draft_opnames} stok opname belum diposting`, 'Lanjutkan', '#/opnames')
    add(data.no_barcode > 0, 'tag', `${data.no_barcode} produk belum punya barcode`, 'Buka produk', '#/products')
    return items
  })

  api<Recap>('/api/recap').then((r) => (recap = r))
  api<Dashboard>('/api/dashboard').then((d) => (data = d))
</script>

<div class="page-head">
  <div class="titles">
    <h1>Halo, {session.user?.full_name}</h1>
    <p>{longDate.format(new Date())}</p>
  </div>
  <a class="button primary" href="#/pos"><Icon name="register" /> Buka layar kasir</a>
</div>

<div class="stats">
  <div class="key">Penjualan hari ini<b>Rp {rupiah(recap?.total ?? 0)}</b></div>
  <div>Transaksi<b>{recap?.transactions ?? 0}</b></div>
  <div>Tunai<b>Rp {rupiah(recap?.cash ?? 0)}</b></div>
  <div>Debit<b>Rp {rupiah(recap?.debit ?? 0)}</b></div>
  <div>QRIS<b>Rp {rupiah(recap?.qris ?? 0)}</b></div>
</div>

<div class="tasks">
  {#each tasks as t}
    <a class="task" href={t.href}>
      <Icon name={t.icon} size={22} />
      <strong>{t.title}</strong>
      <span>{t.text}</span>
    </a>
  {/each}
</div>

<div class="grid">
  <section class="card todo">
    <h2>Perlu perhatian</h2>
    {#each todo as item}
      <a class="item" href={item.href}>
        <Icon name={item.icon} />
        <span>{item.text}</span>
        <em>{item.action} →</em>
      </a>
    {:else}
      <p class="muted">{data ? 'Tidak ada yang perlu ditindak. Semua beres.' : 'Memuat…'}</p>
    {/each}
  </section>

  <section class="card">
    <h2>7 hari terakhir <small>Rp {rupiah(weekTotal)}</small></h2>
    <div class="week">
      {#each data?.week ?? [] as d (d.day)}
        <div class="day" title="Rp {rupiah(d.gross)}">
          <span class="amount">{d.gross ? rupiah(Math.round(d.gross / 1000)) + 'rb' : ''}</span>
          <span class="bar" style="height: {Math.max((d.gross / peak) * 100, 2)}%"></span>
          <span class="label">{weekday.format(new Date(d.day))}</span>
        </div>
      {/each}
    </div>
  </section>

  <section class="card">
    <h2>Stok menipis</h2>
    {#each data?.low_stock ?? [] as p}
      <div class="line"><span>{p.name}</span><b class:neg={p.count <= 0}>{p.count}</b></div>
    {:else}
      <p class="muted">Tidak ada produk aktif yang stoknya tinggal sedikit.</p>
    {/each}
  </section>

  <section class="card">
    <h2>Terlaris 7 hari</h2>
    {#each data?.top_products ?? [] as p, i}
      <div class="line"><span><i>{i + 1}</i>{p.name}</span><b>{p.count} pcs</b></div>
    {:else}
      <p class="muted">Belum ada penjualan minggu ini.</p>
    {/each}
  </section>
</div>

<style>
  .tasks {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 0.8rem;
    margin-bottom: 1.1rem;
  }
  .task {
    display: grid;
    gap: 0.25rem;
    padding: 0.9rem 1.1rem;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--card);
    color: inherit;
    text-decoration: none;
    transition: border-color 0.12s;
  }
  .task:hover {
    border-color: var(--accent);
  }
  .task :global(svg) {
    color: var(--accent);
  }
  .task strong {
    font-weight: 500;
  }
  .task span {
    font-size: 0.86rem;
    color: var(--muted);
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1rem;
  }
  .card {
    padding: 1.1rem 1.2rem;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--card);
  }
  h2 {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    margin-bottom: 0.8rem;
    font-size: 1.1rem;
  }
  h2 small {
    font-family: var(--mono);
    font-size: 0.8rem;
    font-weight: 400;
    color: var(--muted);
  }
  .item {
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    gap: 0.7rem;
    padding: 0.6rem 0.7rem;
    margin: 0 -0.7rem;
    border-radius: 8px;
    color: inherit;
    text-decoration: none;
  }
  .item:hover {
    background: var(--accent-wash);
  }
  .item :global(svg) {
    color: var(--amber);
  }
  .item em {
    font-style: normal;
    font-weight: 500;
    color: var(--accent);
    white-space: nowrap;
  }
  .week {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: 0.5rem;
    height: 150px;
  }
  .day {
    display: grid;
    grid-template-rows: auto 1fr auto;
    justify-items: center;
    align-items: end;
    gap: 0.3rem;
  }
  .bar {
    width: 70%;
    border-radius: 4px 4px 0 0;
    background: var(--accent);
    opacity: 0.8;
  }
  .day:last-child .bar {
    background: var(--amber);
    opacity: 1;
  }
  .amount,
  .label {
    font-family: var(--mono);
    font-size: 0.68rem;
    color: var(--muted);
  }
  .line {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.45rem 0;
    border-bottom: 1px dashed var(--line-strong);
  }
  .line:last-child {
    border-bottom: 0;
  }
  .line b {
    font-family: var(--mono);
    font-weight: 500;
    white-space: nowrap;
  }
  .line i {
    display: inline-block;
    width: 1.4rem;
    font-family: var(--mono);
    font-style: normal;
    color: var(--muted);
  }
  p.muted {
    margin: 0;
  }
  @media (max-width: 860px) {
    .tasks {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    .grid {
      grid-template-columns: 1fr;
    }
  }
</style>
