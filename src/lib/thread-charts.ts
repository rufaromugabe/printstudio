export type ThreadBrand = "none" | "madeira" | "isacord";
export type ThreadSwatch = { code: string; name: string; hex: string };

/** Curated production swatches for nearest-match mapping (not full manufacturer catalogues). */
export const MADEIRA_RAYON: ThreadSwatch[] = [
  { code: "1000", name: "Black", hex: "#1a1a1a" },
  { code: "1001", name: "White", hex: "#f5f5f5" },
  { code: "1147", name: "Navy", hex: "#1b2a4a" },
  { code: "1169", name: "Royal Blue", hex: "#1f4b99" },
  { code: "1297", name: "Sky Blue", hex: "#6aa9d8" },
  { code: "1377", name: "Teal", hex: "#0d6e6e" },
  { code: "1249", name: "Kelly Green", hex: "#1f7a3a" },
  { code: "1174", name: "Forest", hex: "#1f3d2a" },
  { code: "1637", name: "Red", hex: "#c41e3a" },
  { code: "1786", name: "Burgundy", hex: "#6e1a2a" },
  { code: "1874", name: "Orange", hex: "#e36c1a" },
  { code: "1924", name: "Gold", hex: "#d4a017" },
  { code: "1951", name: "Yellow", hex: "#f0d23c" },
  { code: "1110", name: "Purple", hex: "#5b2c6f" },
  { code: "1114", name: "Lavender", hex: "#9b7bb8" },
  { code: "1800", name: "Pink", hex: "#e58bb6" },
  { code: "1840", name: "Hot Pink", hex: "#d6246e" },
  { code: "1220", name: "Brown", hex: "#5c3a21" },
  { code: "1227", name: "Tan", hex: "#c4a574" },
  { code: "1087", name: "Gray", hex: "#7a7a7a" },
];

export const ISACORD: ThreadSwatch[] = [
  { code: "0020", name: "Black", hex: "#141414" },
  { code: "0015", name: "White", hex: "#f7f7f7" },
  { code: "3344", name: "Navy", hex: "#16233f" },
  { code: "3900", name: "Royal", hex: "#204a9a" },
  { code: "3810", name: "Light Blue", hex: "#74b0db" },
  { code: "5220", name: "Teal", hex: "#0f6f6a" },
  { code: "5411", name: "Green", hex: "#218044" },
  { code: "5335", name: "Dark Green", hex: "#1c3a28" },
  { code: "1902", name: "Red", hex: "#c21830" },
  { code: "2113", name: "Maroon", hex: "#6a1828" },
  { code: "1311", name: "Orange", hex: "#e05f12" },
  { code: "0703", name: "Gold", hex: "#cfa016" },
  { code: "0310", name: "Yellow", hex: "#efcf35" },
  { code: "2711", name: "Purple", hex: "#56306d" },
  { code: "2830", name: "Lilac", hex: "#9a7ab5" },
  { code: "2155", name: "Pink", hex: "#e48ab4" },
  { code: "2220", name: "Fuchsia", hex: "#d01f6a" },
  { code: "1154", name: "Brown", hex: "#5a3820" },
  { code: "0870", name: "Khaki", hex: "#c2a36f" },
  { code: "0108", name: "Gray", hex: "#767676" },
];

export function chartFor(brand: ThreadBrand): ThreadSwatch[] {
  if (brand === "madeira") return MADEIRA_RAYON;
  if (brand === "isacord") return ISACORD;
  return [];
}

export function nearestThread(hex: string, brand: ThreadBrand): { hex: string; label: string } {
  const chart = chartFor(brand);
  if (!chart.length) return { hex: normalizeHex(hex), label: normalizeHex(hex) };
  const target = rgb(hex);
  let best = chart[0], bestDist = Infinity;
  for (const swatch of chart) {
    const d = dist(target, rgb(swatch.hex));
    if (d < bestDist) { bestDist = d; best = swatch; }
  }
  const brandName = brand === "madeira" ? "Madeira" : "Isacord";
  return { hex: best.hex, label: `${brandName} ${best.code} ${best.name}` };
}

function normalizeHex(value: string) {
  const v = value.trim();
  if (/^#[0-9a-fA-F]{6}$/.test(v)) return v.toLowerCase();
  if (/^#[0-9a-fA-F]{3}$/.test(v)) {
    const r = v[1], g = v[2], b = v[3];
    return `#${r}${r}${g}${g}${b}${b}`.toLowerCase();
  }
  return "#222222";
}

function rgb(hex: string) {
  const h = normalizeHex(hex).slice(1);
  return { r: parseInt(h.slice(0, 2), 16), g: parseInt(h.slice(2, 4), 16), b: parseInt(h.slice(4, 6), 16) };
}

function dist(a: { r: number; g: number; b: number }, b: { r: number; g: number; b: number }) {
  const dr = a.r - b.r, dg = a.g - b.g, db = a.b - b.b;
  return Math.sqrt(dr * dr + dg * dg + db * db);
}
