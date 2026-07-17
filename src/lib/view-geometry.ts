/** Keep editor canvas aspect locked to physical print size so stage and export share one scale. */

export type ViewSize = {
  canvasWidth: number;
  canvasHeight: number;
  physicalWidthMm: number;
  physicalHeightMm: number;
};

export type BoxElement = {
  x: number;
  y: number;
  w: number;
  h: number;
  fontSize: number;
  letterSpacing?: number;
  strokeWidth?: number;
  curveRadius?: number;
};

export function alignViewCanvas<T extends ViewSize>(view: T): T {
  const physW = Math.max(1, view.physicalWidthMm);
  const physH = Math.max(1, view.physicalHeightMm);
  const physAspect = physW / physH;
  const canvasAspect = view.canvasWidth / Math.max(1, view.canvasHeight);
  if (Math.abs(canvasAspect - physAspect) < 0.002) return view;
  const canvasHeight = Math.max(50, Math.round(view.canvasHeight));
  const canvasWidth = Math.max(50, Math.round(canvasHeight * physAspect));
  return {...view, canvasWidth, canvasHeight};
}

export function remapElementBox<T extends BoxElement>(
  element: T,
  from: Pick<ViewSize, "canvasWidth" | "canvasHeight">,
  to: Pick<ViewSize, "canvasWidth" | "canvasHeight">,
): T {
  if (from.canvasWidth === to.canvasWidth && from.canvasHeight === to.canvasHeight) return element;
  const sx = to.canvasWidth / Math.max(1, from.canvasWidth);
  const sy = to.canvasHeight / Math.max(1, from.canvasHeight);
  return {
    ...element,
    x: element.x * sx,
    y: element.y * sy,
    w: element.w * sx,
    h: element.h * sy,
    fontSize: element.fontSize * sy,
    letterSpacing: element.letterSpacing != null ? element.letterSpacing * sx : element.letterSpacing,
    strokeWidth: element.strokeWidth != null ? element.strokeWidth * sy : element.strokeWidth,
    curveRadius: element.curveRadius != null ? element.curveRadius * ((sx + sy) / 2) : element.curveRadius,
  };
}

export function viewCanvasSignature(view: Pick<ViewSize, "canvasWidth" | "canvasHeight">): string {
  return `${Math.round(view.canvasWidth)}x${Math.round(view.canvasHeight)}`;
}
