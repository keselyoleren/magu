<script lang="ts">
  import { api } from '../../lib/api'
  import { dateTime, rupiah } from '../../lib/format'

  interface Movement {
    id: number
    created_at: string
    reason: 'receipt' | 'sale' | 'void' | 'opname' | 'vendor_return'
    qty_change: number
    ref_id: number | null
    user: string
    note: string
  }

  interface Card {
    name: string
    stock: number
    movements: Movement[]
  }

  const reasons = {
    receipt: 'Penerimaan',
    sale: 'Penjualan',
    void: 'Batal jual',
    opname: 'Stok opname',
    vendor_return: 'Retur ke penitip',
  }

  let { productId }: { productId: number } = $props()
  let card = $state<Card | null>(null)

  const rows = $derived.by(() => {
    if (!card) return []
    let balance = card.stock
    return card.movements.map((m) => {
      const row = { ...m, balance }
      balance -= m.qty_change
      return row
    })
  })

  $effect(() => {
    api<Card>(`/api/products/${productId}/movements`).then((c) => (card = c))
  })
</script>

{#if card}
  <div class="head">
    <strong>{card.name}</strong>
    <span><b>{rupiah(card.stock)}</b> stok sekarang</span>
  </div>
  <div class="list">
    {#each rows as m (m.id)}
      <div class="row">
        <div>
          <strong>{reasons[m.reason]}{m.ref_id ? ` #${m.ref_id}` : ''}</strong>
          <small>{dateTime(m.created_at)}{m.user ? ` · ${m.user}` : ''}</small>
          {#if m.note}<small>{m.note}</small>{/if}
        </div>
        <span class="qty" class:neg={m.qty_change < 0} class:pos={m.qty_change > 0}>
          {m.qty_change > 0 ? '+' : ''}{rupiah(m.qty_change)}
        </span>
        <span class="balance">{rupiah(m.balance)}</span>
      </div>
    {:else}
      <p class="muted">Belum ada mutasi stok.</p>
    {/each}
  </div>
{/if}

<style>
  .head {
    display: grid;
    gap: 0.3rem;
    padding: 1.1rem 1.4rem;
    border-bottom: 1px solid var(--line);
    background: #faf7ef;
  }
  .head b {
    font-family: var(--mono);
    font-size: 1.5rem;
  }
  .head span {
    color: var(--muted);
  }
  .list {
    flex: 1;
    overflow-y: auto;
    padding: 0.4rem 1.4rem 1.4rem;
  }
  .row {
    display: grid;
    grid-template-columns: 1fr 4.5rem 4.5rem;
    align-items: center;
    gap: 0.6rem;
    padding: 0.7rem 0;
    border-bottom: 1px dashed var(--line-strong);
  }
  .row div {
    display: grid;
  }
  small {
    color: var(--muted);
  }
  .qty,
  .balance {
    font-family: var(--mono);
    text-align: right;
  }
  .balance {
    color: var(--muted);
  }
  p {
    margin-top: 1.2rem;
  }
</style>
