<script lang="ts" module>
  const paths = {
    box: 'M3 7.5 12 3l9 4.5v9L12 21l-9-4.5v-9Z M3 7.5 12 12l9-4.5 M12 12v9',
    users:
      'M16 19v-1.5a3.5 3.5 0 0 0-3.5-3.5h-5A3.5 3.5 0 0 0 4 17.5V19 M10 10.5a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z M20 19v-1.5a3.5 3.5 0 0 0-2.5-3.35 M15.5 4.7a3 3 0 0 1 0 5.6',
    search: 'M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14Z M20 20l-4-4',
    plus: 'M12 5v14 M5 12h14',
    upload: 'M12 16V4 M7 9l5-5 5 5 M4 20h16',
    logout: 'M15 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3 M10 16l-4-4 4-4 M6 12h10',
    menu: 'M4 7h16 M4 12h16 M4 17h16',
    close: 'M6 6l12 12 M18 6 6 18',
    inbox: 'M4 13l2.5-8h11L20 13 M4 13v5a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-5 M4 13h4.5a3.5 3.5 0 0 0 7 0H20',
    clipboard:
      'M9 4h6v3H9z M9 5.5H6.5A1.5 1.5 0 0 0 5 7v12a1.5 1.5 0 0 0 1.5 1.5h11A1.5 1.5 0 0 0 19 19V7a1.5 1.5 0 0 0-1.5-1.5H15 M8.5 12.5l2 2 4-4.5',
    trash: 'M5 7h14 M10 7V4.5h4V7 M7 7l.8 12a1 1 0 0 0 1 1h6.4a1 1 0 0 0 1-1L17 7',
    back: 'M19 12H5 M11 6l-6 6 6 6',
    minus: 'M5 12h14',
    home: 'M4 11 12 4l8 7 M6 10v9h12v-9 M10 19v-5h4v5',
    edit: 'M4 20h4L19 9l-4-4L4 16v4Z M13.5 6.5l4 4',
    plug: 'M9 3v5 M15 3v5 M6 8h12v4a6 6 0 0 1-12 0V8Z M12 18v3',
    key: 'M15 7a4 4 0 1 1-3.9 5H8v2H6v2H3v-3l6.1-6.1A4 4 0 0 1 15 7Z M16 8h.01',
    undo: 'M9 14 4 9l5-5 M4 9h10a6 6 0 0 1 0 12h-3',
    tag: 'M3 12V4h8l10 10-8 8L3 12Z M7.5 8.5h.01',
    layers: 'M12 3 3 8l9 5 9-5-9-5Z M3 13l9 5 9-5 M3 17.5l9 5 9-5',
    clock: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Z M12 7v5l3 2',
    wallet:
      'M4 7h14a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V7Z M4 7V6a2 2 0 0 1 2-2h10v3 M16 13.5h.01',
    chart: 'M4 20V4 M4 20h16 M8 16v-4 M12 16V8 M16 16v-6',
    download: 'M12 4v12 M7 11l5 5 5-5 M4 20h16',
    receipt: 'M6 3h12v18l-3-2-3 2-3-2-3 2V3Z M9 8h6 M9 12h6',
    register: 'M4 10h16v9H4z M7 10V5h7v5 M8 14h.01 M12 14h.01 M16 14h.01',
    sliders: 'M4 7h10 M18 7h2 M14 5v4 M4 17h2 M10 17h10 M10 15v4',
    printer:
      'M7 8V4h10v4 M7 17H5a1 1 0 0 1-1-1v-6a1 1 0 0 1 1-1h14a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1h-2 M7 14h10v6H7z',
  }

  export type IconName = keyof typeof paths
</script>

<script lang="ts">
  let { name, size = 18 }: { name: IconName; size?: number } = $props()
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="1.7"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  <path d={paths[name]} />
</svg>
