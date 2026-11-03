#!/usr/bin/env bash
# Menjadikan 50 x 20 mm ukuran bawaan printer label di macOS/Linux (CUPS).
# Layar label tidak mengirim ukuran kertas (lihat Labels.svelte), jadi kertasnya mengikuti driver;
# selama driver masih memakai ukuran lain (mis. 4 x 6 inci) printer tetap menarik kertas sepanjang itu.
# Pemakaian: deploy/label-paper.sh [nama-printer]   (tanpa nama: printer bawaan)
set -euo pipefail

printer=${1:-$(lpstat -d | sed 's/.*: //')}
ppd=/etc/cups/ppd/$printer.ppd
[ -r "$ppd" ] || { echo "Driver printer '$printer' tidak ditemukan di $ppd" >&2; exit 1; }
if grep -q w142h57 "$ppd"; then
  echo "Printer '$printer' sudah memakai ukuran 50 x 20 mm."
  exit 0
fi

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

# Margin nol: HWMargins bawaan driver memakan 2 mm atas dan bawah, terlalu banyak untuk label setinggi 20 mm.
perl -pe '
  BEGIN {
    $page = q{"<</PageSize[141.73 56.69]/ImagingBBox null>>setpagedevice"};
    %size = (PageSize => $page, PageRegion => $page, ImageableArea => q{"0 0 141.73 56.69"}, PaperDimension => q{"141.73 56.69"});
  }
  s/^\*Default(PageSize|PageRegion|ImageableArea|PaperDimension): .*/*Default$1: w142h57/;
  if (/^\*(PageSize|PageRegion|ImageableArea|PaperDimension) / && !$seen{$1}++) {
    $_ = "*$1 w142h57/50 x 20 mm: $size{$1}\n$_";
  }
' "$ppd" > "$tmp"

cupstestppd -q -W translations "$tmp"
sudo lpadmin -p "$printer" -P "$tmp"
lpoptions -p "$printer" -l | grep '^PageSize'
