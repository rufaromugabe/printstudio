import { api, PolygonPaths } from "./api";
import { digitizeElements, DigitizerElement, DigitizerView } from "./embroidery-digitizer";

type ExportElement=DigitizerElement&{assetId?:string;sourceWidth?:number;sourceHeight?:number;textAlign?:"left"|"center"|"right";lineHeight?:number;strokeColor?:string;strokeWidth?:number;shadow?:boolean};
export type ProductionMethod="DTF"|"Vinyl"|"Screen print"|"Sublimation";
export type ProductionResult={method:ProductionMethod;blob:Blob;fileName:string;mime:string;previewUrl:string;summary:string;warnings:string[];widthMM:number;heightMM:number;pixelWidth?:number;pixelHeight?:number};

export async function prepareProductionExport(method:ProductionMethod,name:string,elements:ExportElement[],view:DigitizerView&{bleedMm?:number},mirrorVinyl=true):Promise<ProductionResult>{
  if(!elements.length)throw new Error("Add at least one design element before exporting.");
  const clean=safeName(name),warnings:string[]=[];
  const capabilities=await api.productionCapabilities().catch(()=>null);
  if(method==="DTF"||method==="Sublimation"){
    const bleed=method==="Sublimation"?(view.bleedMm??3):0,dpi=300,widthMM=view.physicalWidthMm+bleed*2,heightMM=view.physicalHeightMm+bleed*2,pixelWidth=Math.ceil(widthMM/25.4*dpi),pixelHeight=Math.ceil(heightMM/25.4*dpi);
    if(pixelWidth*pixelHeight>36_000_000)throw new Error("Production canvas exceeds the safe 36-megapixel browser limit. Reduce the print area or export through a rendering worker.");
    for(const e of elements)if(e.type==="image"&&e.sourceWidth){const physical=e.w/view.canvasWidth*view.physicalWidthMm,dpiActual=e.sourceWidth/(physical/25.4);if(dpiActual<150)warnings.push(`${layerName(e)} is only ${Math.round(dpiActual)} DPI at its placed size.`);else if(dpiActual<300)warnings.push(`${layerName(e)} is ${Math.round(dpiActual)} DPI; 300 DPI is preferred.`)}
    if(method==="Sublimation"&&!coversCanvas(elements,view))warnings.push("Artwork does not cover the full bleed area; unprinted edges may appear after pressing.");
    const blob=await renderPNG(elements,view,pixelWidth,pixelHeight,bleed);return{method,blob,fileName:`${clean}-${method.toLowerCase().replace(" ","-")}-300dpi.png`,mime:"image/png",previewUrl:URL.createObjectURL(blob),summary:`${pixelWidth} × ${pixelHeight}px at 300 DPI${bleed?` with ${bleed} mm bleed`:""}`,warnings,widthMM,heightMM,pixelWidth,pixelHeight};
  }
  const traced=await digitizeElements(elements,view);
  if(method==="Vinyl"){
    if(!capabilities?.polygonBoolean)throw new Error("Vinyl cut paths require the Clipper2 production backend. Rebuild the API with -tags clipper2 and CGO_ENABLED=1 — approximate contour exports are disabled.");
    const cleaned=await cleanVinylPaths(traced.regions.map(r=>r.geometry.rings),view);
    const tiny=cleaned.filter(path=>{const xs=path.map(p=>p.x),ys=path.map(p=>p.y);return Math.min(Math.max(...xs)-Math.min(...xs),Math.max(...ys)-Math.min(...ys))<1}).length;
    if(tiny)warnings.push(`${tiny} contour(s) contain details under 1 mm that may weed poorly.`);
    const paths=cleaned.map(path=>pathToSVG(path));
    const transform=mirrorVinyl?`translate(${view.physicalWidthMm} 0) scale(-1 1)`:"";
    const weed=`<rect id="weed-box" x="0.5" y="0.5" width="${Math.max(0,view.physicalWidthMm-1)}" height="${Math.max(0,view.physicalHeightMm-1)}" fill="none" stroke="#000" stroke-width="0.15"/>`;
    const svg=svgEnvelope(view,`<g fill="none" stroke="#000" stroke-width="0.15" fill-rule="evenodd" transform="${transform}">${paths.map(d=>`<path d="${d}"/>`).join("")}</g>${weed}`);
    const blob=new Blob([svg],{type:"image/svg+xml"});
    return{method,blob,fileName:`${clean}-vinyl-${mirrorVinyl?"mirrored":"normal"}.svg`,mime:"image/svg+xml",previewUrl:URL.createObjectURL(blob),summary:`Cut-ready SVG · Clipper2 cleaned · ${mirrorVinyl?"mirrored for heat transfer":"not mirrored"} · ${paths.length} paths · weed box`,warnings,widthMM:view.physicalWidthMm,heightMM:view.physicalHeightMm};
  }
  const colors=[...new Set(traced.regions.map(r=>r.threadId))];
  if(colors.length>8)warnings.push(`${colors.length} colours create ${colors.length} screen separations; consider reducing the palette.`);
  if(elements.some(e=>e.type==="image"))warnings.push("Raster artwork is exported as traced solid silhouettes; use Screen Pack ZIP for AM/FM halftone separations.");
  try{
    const matches=await api.productionSpotMatch(colors,6);
    warnings.push(`Spot library matched ${matches.matches.length} colour(s) within ΔE00 ≤ 6.`);
  }catch(error){
    warnings.push(error instanceof Error?error.message:"Spot-colour matching failed for one or more inks.");
  }
  const groups=colors.map((color,i)=>`<g id="separation-${i+1}" data-ink="${escapeXML(color)}" fill="${validColor(color)}" fill-rule="evenodd">${traced.regions.filter(r=>r.threadId===color).map(r=>ringsPath(r.geometry.rings,view)).map(d=>`<path d="${d}"/>`).join("")}</g>`).join("");
  const svg=svgEnvelope(view,groups);const blob=new Blob([svg],{type:"image/svg+xml"});
  return{method,blob,fileName:`${clean}-screen-separations.svg`,mime:"image/svg+xml",previewUrl:URL.createObjectURL(blob),summary:`Layered screen-print SVG · ${colors.length} ink separation${colors.length===1?"":"s"}`,warnings,widthMM:view.physicalWidthMm,heightMM:view.physicalHeightMm};
}

async function cleanVinylPaths(ringsList:{x:number;y:number}[][][],view:DigitizerView):Promise<PolygonPaths>{
  const absolute:PolygonPaths=ringsList.flatMap(rings=>rings.map(ring=>ring.map(p=>({x:p.x+view.physicalWidthMm/2,y:p.y+view.physicalHeightMm/2}))));
  if(!absolute.length)throw new Error("No vinyl contours were produced.");
  let merged:PolygonPaths=[absolute[0]];
  for(let i=1;i<absolute.length;i++){
    const result=await api.productionBoolean(merged,[absolute[i]],"union");
    merged=result.paths;
  }
  // Zero-offset round join normalizes self-intersections through Clipper2 without inventing blade compensation.
  const normalized=await api.productionOffset(merged,0,"round");
  return normalized.paths;
}

function pathToSVG(path:{x:number;y:number}[]){return path.map((p,i)=>`${i?"L":"M"}${p.x.toFixed(3)} ${p.y.toFixed(3)}`).join(" ")+" Z"}

async function renderPNG(elements:ExportElement[],view:DigitizerView,width:number,height:number,bleed:number){const canvas=document.createElement("canvas");canvas.width=width;canvas.height=height;const ctx=canvas.getContext("2d");if(!ctx)throw new Error("Canvas renderer unavailable");const sx=(width-2*bleed/25.4*300)/view.canvasWidth,sy=(height-2*bleed/25.4*300)/view.canvasHeight,offset=bleed/25.4*300;
  for(const e of elements){ctx.save();ctx.translate(offset+(e.x+e.w/2)*sx,offset+(e.y+e.h/2)*sy);ctx.rotate(e.rotation*Math.PI/180);if(e.type==="image"){const image=await loadImage(e.value);ctx.drawImage(image,-e.w*sx/2,-e.h*sy/2,e.w*sx,e.h*sy)}else{drawText(ctx,e,sx,sy)}ctx.restore()}return await new Promise<Blob>((resolve,reject)=>canvas.toBlob(blob=>blob?resolve(blob):reject(new Error("PNG encoding failed")),"image/png"))}
function drawText(ctx:CanvasRenderingContext2D,e:ExportElement,sx:number,sy:number){const font=e.fontSize*sy;ctx.fillStyle=e.color||"#000";ctx.strokeStyle=e.strokeColor??"transparent";ctx.lineWidth=(e.strokeWidth??0)*Math.min(sx,sy)*2;ctx.font=`${e.fontStyle??"normal"} ${e.fontWeight??400} ${font}px ${e.fontFamily??"Arial"}`;ctx.textAlign="center";ctx.textBaseline="middle";if(e.shadow){ctx.shadowColor="#00000055";ctx.shadowBlur=6*Math.min(sx,sy);ctx.shadowOffsetY=3*Math.min(sx,sy)}if(e.curveType!=="circle"){const lines=e.value.split("\n"),line=font*(e.lineHeight??1.1);lines.forEach((text,i)=>paintText(ctx,text,0,(i-(lines.length-1)/2)*line,(e.letterSpacing??0)*sx));return}const chars=[...e.value],radius=(e.curveRadius??85)*Math.min(sx,sy),sweep=(e.curveSweep??240)*Math.PI/180,sign=e.curveDirection==="counterclockwise"?-1:1;chars.forEach((char,i)=>{const t=chars.length===1?.5:i/(chars.length-1),a=-Math.PI/2+sign*(t-.5)*sweep;ctx.save();ctx.translate(Math.cos(a)*radius,Math.sin(a)*radius);ctx.rotate(a+sign*Math.PI/2);paintText(ctx,char,0,0,0);ctx.restore()})}
function paintText(ctx:CanvasRenderingContext2D,text:string,x:number,y:number,spacing:number){if(!spacing){if(ctx.lineWidth>0)ctx.strokeText(text,x,y);ctx.fillText(text,x,y);return}const chars=[...text],widths=chars.map(c=>ctx.measureText(c).width),total=widths.reduce((a,b)=>a+b,0)+spacing*Math.max(0,chars.length-1);let cursor=x-total/2;ctx.textAlign="left";chars.forEach((char,i)=>{if(ctx.lineWidth>0)ctx.strokeText(char,cursor,y);ctx.fillText(char,cursor,y);cursor+=widths[i]+spacing});ctx.textAlign="center"}
function loadImage(src:string){return new Promise<HTMLImageElement>((resolve,reject)=>{const image=new Image();image.crossOrigin="anonymous";image.onload=()=>resolve(image);image.onerror=()=>reject(new Error("An artwork image could not be decoded for production export."));image.src=src})}
function ringsPath(rings:{x:number;y:number}[][],view:DigitizerView){return rings.map(r=>r.map((p,i)=>`${i?"L":"M"}${(p.x+view.physicalWidthMm/2).toFixed(3)} ${(p.y+view.physicalHeightMm/2).toFixed(3)}`).join(" ")+" Z").join(" ")}
function svgEnvelope(view:DigitizerView,body:string){return `<svg xmlns="http://www.w3.org/2000/svg" width="${view.physicalWidthMm}mm" height="${view.physicalHeightMm}mm" viewBox="0 0 ${view.physicalWidthMm} ${view.physicalHeightMm}"><metadata>PrintStudio production export; units=mm</metadata>${body}</svg>`}
function coversCanvas(elements:ExportElement[],view:DigitizerView){return elements.some(e=>e.x<=0&&e.y<=0&&e.x+e.w>=view.canvasWidth&&e.y+e.h>=view.canvasHeight)}
function layerName(e:ExportElement){return e.type==="text"?`Text “${e.value.slice(0,20)}”`:"Uploaded artwork"}
function safeName(name:string){return name.trim().replace(/[^a-z0-9_-]+/gi,"-").replace(/^-|-$/g,"")||"printstudio-design"}
function escapeXML(value:string){return value.replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&apos;"}[c]!))}
function validColor(value:string){return /^#[0-9a-f]{6}$/i.test(value)?value:"#000000"}
