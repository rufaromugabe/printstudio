import { api, PolygonPaths } from "./api";
import { digitizeElements, DigitizerElement, DigitizerView } from "./embroidery-digitizer";

type ExportElement=DigitizerElement&{assetId?:string;sourceWidth?:number;sourceHeight?:number;textAlign?:"left"|"center"|"right";lineHeight?:number;strokeColor?:string;strokeWidth?:number;shadow?:boolean};
export type ProductionMethod="DTF"|"Vinyl"|"Screen print"|"Sublimation";
export type ProductionResult={method:ProductionMethod;blob:Blob;fileName:string;mime:string;previewUrl:string;summary:string;warnings:string[];widthMM:number;heightMM:number;pixelWidth?:number;pixelHeight?:number;sha256?:string;renderer?:"server"|"browser"};

export async function refreshExportElementURLs(elements:ExportElement[]):Promise<ExportElement[]>{
  return Promise.all(elements.map(async element=>{
    if(element.type!=="image"||!element.assetId)return element;
    try{
      const fresh=await api.assetURL(element.assetId);
      return{...element,value:fresh.url};
    }catch{
      return element;
    }
  }));
}

export async function prepareProductionExport(method:ProductionMethod,name:string,elements:ExportElement[],view:DigitizerView&{bleedMm?:number},mirrorVinyl=true):Promise<ProductionResult>{
  if(!elements.length)throw new Error("Add at least one design element before exporting.");
  const clean=safeName(name),warnings:string[]=[];
  elements=await refreshExportElementURLs(elements);
  const capabilities=await api.productionCapabilities().catch(()=>null);
  if(method==="DTF"||method==="Sublimation"){
    const bleed=method==="Sublimation"?(view.bleedMm??3):0,dpi=300,widthMM=view.physicalWidthMm+bleed*2,heightMM=view.physicalHeightMm+bleed*2;
    for(const e of elements)if(e.type==="image"&&e.sourceWidth){const physical=e.w/view.canvasWidth*view.physicalWidthMm,dpiActual=e.sourceWidth/(physical/25.4);if(dpiActual<150)warnings.push(`${layerName(e)} is only ${Math.round(dpiActual)} DPI at its placed size.`);else if(dpiActual<300)warnings.push(`${layerName(e)} is ${Math.round(dpiActual)} DPI; 300 DPI is preferred.`)}
    if(method==="Sublimation"&&!coversCanvas(elements,view))warnings.push("Artwork does not cover the full bleed area; unprinted edges may appear after pressing.");
    const rendered=await api.productionRenderScene({
      name:clean,method,dpi,
      view:{canvasWidth:view.canvasWidth,canvasHeight:view.canvasHeight,physicalWidthMm:view.physicalWidthMm,physicalHeightMm:view.physicalHeightMm,bleedMm:bleed},
      elements:elements.map(e=>({id:e.id,type:e.type,value:e.value,assetId:e.assetId,x:e.x,y:e.y,w:e.w,h:e.h,rotation:e.rotation,color:e.color,fontSize:e.fontSize,fontWeight:e.fontWeight,letterSpacing:e.letterSpacing,lineHeight:e.lineHeight,sourceWidth:e.sourceWidth,sourceHeight:e.sourceHeight})),
    });
    const pixelWidth=rendered.widthPx||Math.ceil(widthMM/25.4*dpi),pixelHeight=rendered.heightPx||Math.ceil(heightMM/25.4*dpi);
    warnings.push("Rendered on the production server (not the browser canvas).");
    return{method,blob:rendered.blob,fileName:`${clean}-${method.toLowerCase().replace(" ","-")}-300dpi.png`,mime:"image/png",previewUrl:URL.createObjectURL(rendered.blob),summary:`Server ${pixelWidth} × ${pixelHeight}px at 300 DPI${bleed?` with ${bleed} mm bleed`:""}`,warnings,widthMM,heightMM,pixelWidth,pixelHeight,sha256:rendered.sha256,renderer:"server"};
  }
  const traced=await digitizeElements(elements,view);
  if(method==="Vinyl"){
    if(!capabilities?.polygonBoolean)throw new Error("Vinyl cut paths require the Clipper2 production backend. Rebuild/deploy the API with -tags clipper2 — approximate contour exports are disabled.");
    const cleaned=await cleanVinylPaths(traced.regions.map(r=>r.geometry.rings),view);
    const tiny=cleaned.filter(path=>{const xs=path.map(p=>p.x),ys=path.map(p=>p.y);return Math.min(Math.max(...xs)-Math.min(...xs),Math.max(...ys)-Math.min(...ys))<1}).length;
    if(tiny)warnings.push(`${tiny} contour(s) contain details under 1 mm that may weed poorly.`);
    const holeCount=Math.max(0,cleaned.length-traced.regions.length);
    if(holeCount>0)warnings.push(`Kept ${holeCount} interior cutout(s) for weeding.`);
    const compound=cleaned.map(path=>pathToSVG(path)).join("");
    const transform=mirrorVinyl?`translate(${view.physicalWidthMm} 0) scale(-1 1)`:"";
    const weed=`<rect id="weed-box" x="0.5" y="0.5" width="${Math.max(0,view.physicalWidthMm-1)}" height="${Math.max(0,view.physicalHeightMm-1)}" fill="none" stroke="#000" stroke-width="0.15"/>`;
    const svg=svgEnvelope(view,`<g transform="${transform}"><path d="${compound}" fill="#111111" fill-opacity="0.16" stroke="#000" stroke-width="0.15" fill-rule="evenodd"/></g>${weed}`);
    const blob=new Blob([svg],{type:"image/svg+xml"});
    const sha256=await hashBlob(blob);
    return{method,blob,fileName:`${clean}-vinyl-${mirrorVinyl?"mirrored":"normal"}.svg`,mime:"image/svg+xml",previewUrl:URL.createObjectURL(blob),summary:`Cut-ready SVG · Clipper2 cleaned · ${mirrorVinyl?"mirrored for heat transfer":"not mirrored"} · ${cleaned.length} contours · weed box`,warnings,widthMM:view.physicalWidthMm,heightMM:view.physicalHeightMm,sha256,renderer:"server"};
  }
  const colors=[...new Set(traced.regions.map(r=>r.threadId))];
  if(colors.length>8)warnings.push(`${colors.length} colours create ${colors.length} screen separations; consider reducing the palette.`);
  if(elements.some(e=>e.type==="image"))warnings.push("Raster artwork is exported as traced solid silhouettes; use Screen Pack ZIP for AM/FM halftone separations.");
  try{
    const matches=await api.productionSpotMatch(colors.filter(c=>/^#[0-9a-fA-F]{6}$/.test(c)),6);
    warnings.push(`Spot library matched ${matches.matches.length} colour(s) within ΔE00 ≤ 6.`);
  }catch(error){
    warnings.push(error instanceof Error?error.message:"Spot-colour matching failed for one or more inks.");
  }
  const groups=colors.map((color,i)=>`<g id="separation-${i+1}" data-ink="${escapeXML(color)}" fill="${validColor(color)}" fill-rule="evenodd">${traced.regions.filter(r=>r.threadId===color).map(r=>ringsPath(r.geometry.rings,view)).map(d=>`<path d="${d}"/>`).join("")}</g>`).join("");
  const svg=svgEnvelope(view,groups);const blob=new Blob([svg],{type:"image/svg+xml"});
  const sha256=await hashBlob(blob);
  return{method,blob,fileName:`${clean}-screen-separations.svg`,mime:"image/svg+xml",previewUrl:URL.createObjectURL(blob),summary:`Layered screen-print SVG · ${colors.length} ink separation${colors.length===1?"":"s"}`,warnings,widthMM:view.physicalWidthMm,heightMM:view.physicalHeightMm,sha256,renderer:"server"};
}

async function cleanVinylPaths(ringsList:{x:number;y:number}[][][],view:DigitizerView):Promise<PolygonPaths>{
  // Each entry is one traced shape: rings[0]=exterior, rings[1+]=holes/cutouts.
  const polygons=ringsList
    .map(rings=>rings.map(ring=>ring.map(p=>({x:p.x+view.physicalWidthMm/2,y:p.y+view.physicalHeightMm/2}))))
    .filter(rings=>rings.length>0&&rings[0].length>=3);
  if(!polygons.length)throw new Error("No vinyl contours were produced.");

  // Carve holes with difference first. A flat union of every ring fills interiors
  // and leaves only the outer silhouette — wrong for vinyl stencils.
  const solids:PolygonPaths=[];
  for(const rings of polygons){
    let paths:PolygonPaths=[rings[0]];
    for(let i=1;i<rings.length;i++){
      if(rings[i].length<3)continue;
      const cut=await api.productionBoolean(paths,[rings[i]],"difference");
      paths=cut.paths;
    }
    solids.push(...paths);
  }
  if(!solids.length)throw new Error("No vinyl contours were produced.");

  let merged:PolygonPaths=[solids[0]];
  for(let i=1;i<solids.length;i++){
    const result=await api.productionBoolean(merged,[solids[i]],"union");
    merged=result.paths;
  }
  const normalized=await api.productionOffset(merged,0,"round");
  return normalized.paths.length?normalized.paths:merged;
}

function pathToSVG(path:{x:number;y:number}[]){return path.map((p,i)=>`${i?"L":"M"}${p.x.toFixed(3)} ${p.y.toFixed(3)}`).join(" ")+" Z"}
function ringsPath(rings:{x:number;y:number}[][],view:DigitizerView){return rings.map(r=>r.map((p,i)=>`${i?"L":"M"}${(p.x+view.physicalWidthMm/2).toFixed(3)} ${(p.y+view.physicalHeightMm/2).toFixed(3)}`).join(" ")+" Z").join(" ")}
function svgEnvelope(view:DigitizerView,body:string){return `<svg xmlns="http://www.w3.org/2000/svg" width="${view.physicalWidthMm}mm" height="${view.physicalHeightMm}mm" viewBox="0 0 ${view.physicalWidthMm} ${view.physicalHeightMm}"><metadata>PrintStudio production export; units=mm</metadata>${body}</svg>`}
function coversCanvas(elements:ExportElement[],view:DigitizerView){return elements.some(e=>e.x<=0&&e.y<=0&&e.x+e.w>=view.canvasWidth&&e.y+e.h>=view.canvasHeight)}
function layerName(e:ExportElement){return e.type==="text"?`Text “${e.value.slice(0,20)}”`:"Uploaded artwork"}
function safeName(name:string){return name.trim().replace(/[^a-z0-9_-]+/gi,"-").replace(/^-|-$/g,"")||"printstudio-design"}
function escapeXML(value:string){return value.replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&apos;"}[c]!))}
function validColor(value:string){return /^#[0-9a-f]{6}$/i.test(value)?value:"#000000"}
async function hashBlob(blob:Blob){const digest=await crypto.subtle.digest("SHA-256",await blob.arrayBuffer());return Array.from(new Uint8Array(digest),b=>b.toString(16).padStart(2,"0")).join("")}
