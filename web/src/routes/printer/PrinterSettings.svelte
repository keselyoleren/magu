<script lang="ts">
  import Icon from '../../components/Icon.svelte'
  import { isPaired, pairPrinter, printerConfig, savePrinterConfig, supported, type Transport } from '../../lib/printer'
  import { printTest } from '../../lib/receipt'

  const saved = printerConfig()
  let transport = $state<Transport>(saved.transport)
  let baudRate = $state(saved.baudRate)
  let paired = $state(false)
  let message = $state('')
  let failed = $state(false)
  let busy = $state(false)

  const needsPairing = $derived(transport === 'serial' || transport === 'usb')
  const blocked = $derived(!supported(transport))

  function tell(text: string, isError = false) {
    message = text
    failed = isError
  }

  async function changed() {
    await savePrinterConfig({ transport, baudRate })
    paired = await isPaired()
    tell('')
  }

  async function run(action: () => Promise<void>, done: string) {
    busy = true
    try {
      await action()
      tell(done)
    } catch (err) {
      tell((err as Error).message, true)
    } finally {
      paired = await isPaired()
      busy = false
    }
  }

  isPaired().then((p) => (paired = p))
</script>

<div class="page-head">
  <div class="titles">
    <h1>Printer</h1>
    <p>Pengaturan ini disimpan di komputer yang sedang dipakai, jadi tiap kasir mengaturnya sendiri sekali</p>
  </div>
</div>

<div class="form-card">
  <div class="form-body">
    <label>
      Printer struk tercolok di
      <select bind:value={transport} onchange={changed}>
        <option value="browser">Cetak biasa lewat browser (disarankan)</option>
        <option value="serial">Lanjutan: langsung ke printer, port serial / COM</option>
        <option value="usb">Lanjutan: langsung ke printer, USB</option>
        <option value="server">Lanjutan: printer tercolok di server</option>
      </select>
    </label>

    {#if transport === 'browser'}
      <p class="muted">
        Struk dicetak seperti mencetak halaman biasa, ke printer yang terpasang di komputer ini. Tidak ada yang perlu
        dihubungkan: cukup pasang driver printernya dan jadikan printer bawaan.
      </p>
      <ul class="muted">
        <li>Ukuran kertas struk (57 mm atau 80 mm) dan margin dikirim aplikasi ke browser; di dialog cetak biarkan semua pilihan pada "Default".</li>
        <li>Label mengikuti kertas driver: jadikan 50 × 20 mm ukuran bawaan printer label.</li>
        <li>Supaya dialog tidak muncul tiap transaksi, buka Chrome/Edge lewat pintasan dengan tambahan <code>--kiosk-printing</code>.</li>
        <li>Laci uang dibuka oleh driver printer: aktifkan "Open cash drawer" di pengaturan printernya.</li>
      </ul>
    {:else if transport === 'serial'}
      <label>
        Kecepatan (baud)
        <select bind:value={baudRate} onchange={changed}>
          {#each [9600, 19200, 38400, 57600, 115200] as rate}
            <option value={rate}>{rate}</option>
          {/each}
        </select>
      </label>
      <p class="muted">
        Untuk printer yang muncul sebagai port COM (Windows) atau <code>/dev/ttyUSB</code>, termasuk printer Bluetooth
        yang sudah dipasangkan. Kalau hasil cetak berupa karakter acak, coba kecepatan lain.
      </p>
    {:else if transport === 'usb'}
      <p class="muted">
        Untuk printer USB tanpa port COM. Di Windows, driver printer harus diganti ke WinUSB (dengan aplikasi Zadig) dan
        printer itu tidak bisa lagi dipakai aplikasi Windows lain. Di Linux, printer tidak boleh sedang dipakai sistem
        (CUPS atau modul <code>usblp</code>).
      </p>
    {:else}
      <p class="muted">Struk dikirim ke server dan dicetak oleh printer yang tercolok di sana.</p>
    {/if}

    {#if blocked}
      <p class="error">
        Browser ini tidak memberi akses ke printer. Pakai Chrome atau Edge, dan buka aplikasi lewat alamat
        <strong>https://</strong>; lewat http biasa browser menutup akses ke perangkat USB.
      </p>
    {:else}
      <p class="state" class:ok={paired}>
        <Icon name="plug" />
        {#if needsPairing}
          {paired ? 'Printer sudah diizinkan di browser ini.' : 'Belum ada printer yang dihubungkan.'}
        {:else}
          {transport === 'browser' ? 'Siap. Tekan Tes cetak untuk mencoba.' : paired ? 'Server punya printer.' : 'Printer belum diatur di server.'}
        {/if}
      </p>
    {/if}

    {#if message}<p class:error={failed} class="muted">{message}</p>{/if}
  </div>
  <div class="form-actions">
    <button disabled={busy || blocked || !paired} onclick={() => run(printTest, 'Tes cetak terkirim.')}>
      <Icon name="printer" /> Tes cetak
    </button>
    {#if needsPairing}
      <button class="primary" disabled={busy || blocked} onclick={() => run(pairPrinter, 'Printer terhubung.')}>
        <Icon name="plug" /> {paired ? 'Pilih printer lain' : 'Hubungkan printer'}
      </button>
    {/if}
  </div>
</div>

<style>
  p,
  ul {
    margin: 0;
  }
  ul {
    padding-left: 1.1rem;
  }
  code {
    font-family: var(--mono);
    font-size: 0.92em;
  }
  .state {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.6rem 0.8rem;
    border-radius: 7px;
    background: #f6ecd2;
    color: #7a5608;
  }
  .state.ok {
    background: var(--accent-wash);
    color: var(--accent-dark);
  }
</style>
