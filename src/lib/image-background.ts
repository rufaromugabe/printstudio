/** Heuristic solid-background removal for logo-style artwork (not ML subject cutout). */

export type BackgroundRemovalResult = {
  removed: boolean;
  reason: "already-transparent" | "no-uniform-edge" | "unsafe-coverage" | "removed" | "candidate";
  removedPixels: number;
};

type RGB = { r: number; g: number; b: number };

const ALPHA_OPAQUE = 32;
const EDGE_MATCH_MIN = 0.72;
const COLOR_TOLERANCE = 36;
const MAX_REMOVE_RATIO = 0.92;
const MIN_REMOVE_RATIO = 0.04;

export function removeSolidBackground(
  data: Uint8ClampedArray,
  width: number,
  height: number,
  options?: { tolerance?: number; dryRun?: boolean },
): BackgroundRemovalResult {
  const total = width * height;
  if (total === 0) return { removed: false, reason: "no-uniform-edge", removedPixels: 0 };

  let transparent = 0;
  for (let i = 0; i < total; i++) if (data[i * 4 + 3] < ALPHA_OPAQUE) transparent++;
  if (transparent / total >= 0.08) {
    return { removed: false, reason: "already-transparent", removedPixels: 0 };
  }

  const tolerance = options?.tolerance ?? COLOR_TOLERANCE;
  const edgeIndexes = borderIndexes(width, height);
  const samples: RGB[] = [];
  for (const idx of edgeIndexes) {
    const o = idx * 4;
    if (data[o + 3] < ALPHA_OPAQUE) continue;
    samples.push({ r: data[o], g: data[o + 1], b: data[o + 2] });
  }
  if (samples.length < 8) return { removed: false, reason: "no-uniform-edge", removedPixels: 0 };

  const bg = averageColor(samples);
  let edgeMatches = 0;
  for (const sample of samples) if (colorDistance(sample, bg) <= tolerance) edgeMatches++;
  if (edgeMatches / samples.length < EDGE_MATCH_MIN) {
    return { removed: false, reason: "no-uniform-edge", removedPixels: 0 };
  }

  const visited = new Uint8Array(total);
  const queue = new Int32Array(total);
  let head = 0;
  let tail = 0;
  const enqueue = (idx: number) => {
    if (visited[idx]) return;
    const o = idx * 4;
    if (data[o + 3] < ALPHA_OPAQUE) return;
    if (colorDistance({ r: data[o], g: data[o + 1], b: data[o + 2] }, bg) > tolerance) return;
    visited[idx] = 1;
    queue[tail++] = idx;
  };

  for (const idx of edgeIndexes) enqueue(idx);

  while (head < tail) {
    const idx = queue[head++];
    const x = idx % width;
    const y = (idx / width) | 0;
    if (x > 0) enqueue(idx - 1);
    if (x + 1 < width) enqueue(idx + 1);
    if (y > 0) enqueue(idx - width);
    if (y + 1 < height) enqueue(idx + width);
  }

  const removedPixels = tail;
  const ratio = removedPixels / total;
  if (ratio < MIN_REMOVE_RATIO || ratio > MAX_REMOVE_RATIO) {
    return { removed: false, reason: "unsafe-coverage", removedPixels: 0 };
  }

  if (options?.dryRun) {
    return { removed: false, reason: "candidate", removedPixels };
  }

  for (let i = 0; i < removedPixels; i++) {
    data[queue[i] * 4 + 3] = 0;
  }
  return { removed: true, reason: "removed", removedPixels };
}

export async function inspectImageBackground(src: string): Promise<BackgroundRemovalResult> {
  const { imageData, width, height } = await rasterizeImage(src);
  return removeSolidBackground(imageData.data, width, height, { dryRun: true });
}

export async function cleanImageBackground(src: string, fileName = "artwork-no-bg.png"): Promise<File | null> {
  const { canvas, imageData, width, height } = await rasterizeImage(src);
  const result = removeSolidBackground(imageData.data, width, height);
  if (!result.removed) return null;
  const ctx = canvas.getContext("2d");
  if (!ctx) return null;
  ctx.putImageData(imageData, 0, 0);
  const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, "image/png"));
  if (!blob) return null;
  return new File([blob], fileName, { type: "image/png" });
}

async function rasterizeImage(src: string): Promise<{ canvas: HTMLCanvasElement; imageData: ImageData; width: number; height: number }> {
  const image = await loadImage(src);
  const maxEdge = 1600;
  const scale = Math.min(1, maxEdge / Math.max(image.naturalWidth || image.width, image.naturalHeight || image.height));
  const width = Math.max(1, Math.round((image.naturalWidth || image.width) * scale));
  const height = Math.max(1, Math.round((image.naturalHeight || image.height) * scale));
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d", { willReadFrequently: true });
  if (!ctx) throw new Error("canvas unavailable");
  ctx.drawImage(image, 0, 0, width, height);
  const imageData = ctx.getImageData(0, 0, width, height);
  return { canvas, imageData, width, height };
}

function loadImage(src: string) {
  return new Promise<HTMLImageElement>((resolve, reject) => {
    const image = new Image();
    image.crossOrigin = "anonymous";
    image.onload = () => resolve(image);
    image.onerror = () => reject(new Error("image decode failed"));
    image.src = src;
  });
}

function borderIndexes(width: number, height: number): number[] {
  const out: number[] = [];
  for (let x = 0; x < width; x++) {
    out.push(x);
    if (height > 1) out.push((height - 1) * width + x);
  }
  for (let y = 1; y < height - 1; y++) {
    out.push(y * width);
    out.push(y * width + (width - 1));
  }
  return out;
}

function averageColor(samples: RGB[]): RGB {
  let r = 0, g = 0, b = 0;
  for (const sample of samples) {
    r += sample.r;
    g += sample.g;
    b += sample.b;
  }
  const n = samples.length;
  return { r: r / n, g: g / n, b: b / n };
}

function colorDistance(a: RGB, b: RGB): number {
  const dr = a.r - b.r, dg = a.g - b.g, db = a.b - b.b;
  return Math.sqrt(dr * dr + dg * dg + db * db);
}
