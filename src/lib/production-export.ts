import { api, PolygonPaths, VinylMaterialClass, VinylReview } from "./api";
import { digitizeElements, DigitizerElement, DigitizerView } from "./embroidery-digitizer";

type ExportElement=DigitizerElement&{assetId?:string;sourceWidth?:number;sourceHeight?:number;textAlign?:"left"|"center"|"right";lineHeight?:number;strokeColor?:string;strokeWidth?:number;shadow?:boolean};
export type ProductionMethod="DTF"|"Vinyl"|"Screen print"|"Sublimation";
export type ProductionResult={method:ProductionMethod;blob:Blob;fileName:string;mime:string;previewUrl:string;summary:string;warnings:string[];widthMM:number;heightMM:number;pixelWidth?:number;pixelHeight?:number;sha256?:string;renderer?:"server"|"browser";vinylReview?:VinylReview;vinylBlocked?:boolean};
export type VinylExportOptions={mirror?:boolean;materialClass?:VinylMaterialClass|string;advancedVectorize?:boolean};
export type ContourExportOptions={mirror?:boolean;materialClass?:VinylMaterialClass|string;advancedVectorize?:boolean};

const CONTENT_MARGIN_MM=1.5;
const DEFAULT_VINYL_MATERIAL:VinylMaterialClass="htv-smooth";

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

export async function prepareProductionExport(method:ProductionMethod,name:string,elements:ExportElement[],view:DigitizerView&{bleedMm?:number},mirrorOrOptions:boolean|ContourExportOptions=true):Promise<ProductionResult>{
  if(!elements.length)throw new Error("Add at least one design element before exporting.");
  const vinylOpts:ContourExportOptions=typeof mirrorOrOptions==="boolean"?{mirror:mirrorOrOptions}:{...mirrorOrOptions};
  const mirrorVinyl=vinylOpts.mirror??true;
  const materialClass=(vinylOpts.materialClass||DEFAULT_VINYL_MATERIAL) as VinylMaterialClass;
  const clean=safeName(name),warnings:string[]=[];
  elements=await refreshExportElementURLs(elements);
  const capabilities=await api.productionCapabilities().catch(()=>null);
  if(method==="DTF"||method==="Sublimation"){
    const bleed=method==="Sublimation"?(view.bleedMm??3):0,dpi=300;
    // Keep full print-area framing so preview placement matches the shirt mockup.
    // (Cropping to ink made artwork look centered/zoomed and broke alignment.)
    const cropToContent=false;
    const fullWidthMM=view.physicalWidthMm+bleed*2,fullHeightMM=view.physicalHeightMm+bleed*2;
    for(const e of elements)if(e.type==="image"&&e.sourceWidth){const physical=e.w/view.canvasWidth*view.physicalWidthMm,dpiActual=e.sourceWidth/(physical/25.4);if(dpiActual<150)warnings.push(`${layerName(e)} is only ${Math.round(dpiActual)} DPI at its placed size.`);else if(dpiActual<300)warnings.push(`${layerName(e)} is ${Math.round(dpiActual)} DPI; 300 DPI is preferred.`)}
    if(method==="Sublimation"&&!coversCanvas(elements,view))warnings.push("Artwork does not cover the full bleed area; unprinted edges may appear after pressing.");
    const rendered=await api.productionRenderScene({
      name:clean,method,dpi,cropToContent,
      view:{canvasWidth:view.canvasWidth,canvasHeight:view.canvasHeight,physicalWidthMm:view.physicalWidthMm,physicalHeightMm:view.physicalHeightMm,bleedMm:bleed},
      elements:elements.map(e=>({id:e.id,type:e.type,value:e.value,assetId:e.assetId,x:e.x,y:e.y,w:e.w,h:e.h,rotation:e.rotation,color:e.color,fontSize:e.fontSize,fontWeight:e.fontWeight,letterSpacing:e.letterSpacing,lineHeight:e.lineHeight,sourceWidth:e.sourceWidth,sourceHeight:e.sourceHeight})),
    });
    const pixelWidth=rendered.widthPx||Math.ceil(fullWidthMM/25.4*dpi),pixelHeight=rendered.heightPx||Math.ceil(fullHeightMM/25.4*dpi);
    const widthMM=fullWidthMM;
    const heightMM=fullHeightMM;
    warnings.push("Rendered on the production server (not the browser canvas).");
    warnings.push(`Framed to the print area (${widthMM.toFixed(1)} × ${heightMM.toFixed(1)} mm) — same placement as the shirt mockup.`);
    return{method,blob:rendered.blob,fileName:`${clean}-${method.toLowerCase().replace(" ","-")}-300dpi.png`,mime:"image/png",previewUrl:URL.createObjectURL(rendered.blob),summary:`Server ${pixelWidth} × ${pixelHeight}px at 300 DPI · ${widthMM.toFixed(1)} × ${heightMM.toFixed(1)} mm${bleed?` with ${bleed} mm bleed`:""}`,warnings,widthMM,heightMM,pixelWidth,pixelHeight,sha256:rendered.sha256,renderer:"server"};
  }
  if(elements.some(e=>e.type==="image")&&!capabilities?.vectorTrace){
    throw new Error("Image layers require server Potrace vectorize. Install potrace / set POTRACE_BIN on the API.");
  }
  const mlPrep=vinylOpts.advancedVectorize!==false;
  const traced=await digitizeElements(elements,view,{
    mode:method==="Vinyl"?"silhouette":"color",
    method:method==="Vinyl"?"vinyl":"screen",
    mlPrep,
  });
  for(const d of traced.vectorDiagnostics??[]){
    if(d.severity==="error"||d.severity==="warning")warnings.push(`Vectorize ${d.severity}: ${d.message}`);
  }
  for(const report of traced.vectorReports??[]){
    const detected=report.contentKind==="text-like"?"raster text / lettering":report.contentKind==="flat-art"?"logo / flat artwork":"continuous-tone artwork";
    const background=report.backgroundRemoved?" · background isolated":"";
    const upscale=report.upscaleFactor>1?` · ${report.upscaleFactor}× edge supersampling`:"";
    warnings.push(`Auto polish: detected ${detected}${background}${upscale} · ${report.profile} · quality ${report.qualityScore}/100.`);
  }
  if(method==="Vinyl"){
    if(!capabilities?.polygonBoolean)throw new Error("Vinyl cut paths require the Clipper2 production backend. Rebuild/deploy the API with -tags clipper2 — approximate contour exports are disabled.");
    const exteriorCount=traced.regions.length;
    const cleaned=await cleanVinylPaths(traced.regions.map(r=>r.geometry.rings),view);
    const holeCount=Math.max(0,cleaned.length-exteriorCount);
    if(holeCount>0)warnings.push(`Kept ${holeCount} interior cutout(s) for weeding.`);
    const reviewResult=await api.vinylReview({materialClass,paths:cleaned,mirrored:mirrorVinyl});
    for(const d of reviewResult.diagnostics){
      const prefix=d.severity==="error"?"Hard stop":"Warning";
      warnings.push(`${prefix}: ${d.message}${d.pathId?` (${d.pathId})`:""}`);
    }
    warnings.push(`${reviewResult.profile.label} · warn < ${reviewResult.profile.warnFeatureMm.toFixed(1)} mm · reject < ${reviewResult.profile.rejectFeatureMm.toFixed(1)} mm`);
    if(reviewResult.profile.notes)warnings.push(reviewResult.profile.notes);
    const vinylBlocked=reviewResult.review.decision==="blocked"||reviewResult.diagnostics.some(d=>d.severity==="error");
    if(vinylBlocked)warnings.push("Cut SVG download is blocked until hard stops are resolved.");
    const framed=framePathsToContent(cleaned,CONTENT_MARGIN_MM);
    const compound=framed.paths.map(path=>pathToSVG(path)).join("");
    const transform=mirrorVinyl?`translate(${framed.widthMM} 0) scale(-1 1)`:"";
    const weed=`<rect id="weed-box" x="0.5" y="0.5" width="${Math.max(0,framed.widthMM-1)}" height="${Math.max(0,framed.heightMM-1)}" fill="none" stroke="#000" stroke-width="0.15"/>`;
    const svg=svgEnvelope(framed.widthMM,framed.heightMM,`<g transform="${transform}"><path d="${compound}" fill="#111111" fill-opacity="0.55" stroke="#000" stroke-width="0.12" fill-rule="evenodd"/></g>${weed}`);
    const blob=new Blob([svg],{type:"image/svg+xml"});
    const sha256=await hashBlob(blob);
    warnings.push(`Sized to cut contours (${framed.widthMM.toFixed(1)} × ${framed.heightMM.toFixed(1)} mm), not the full print area.`);
    return{method,blob,fileName:`${clean}-vinyl-${mirrorVinyl?"mirrored":"normal"}.svg`,mime:"image/svg+xml",previewUrl:URL.createObjectURL(blob),summary:`Cut-ready SVG · ${reviewResult.profile.label} · Clipper2 cleaned · ${mirrorVinyl?"mirrored for heat transfer":"not mirrored"} · ${cleaned.length} contours · ${framed.widthMM.toFixed(1)} × ${framed.heightMM.toFixed(1)} mm`,warnings,widthMM:framed.widthMM,heightMM:framed.heightMM,sha256,renderer:"server",vinylReview:reviewResult.review,vinylBlocked};
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
  const printPaths=traced.regions.flatMap(r=>r.geometry.rings.map(ring=>ring.map(p=>({x:p.x+view.physicalWidthMm/2,y:p.y+view.physicalHeightMm/2}))));
  const framed=framePathsToContent(printPaths,CONTENT_MARGIN_MM);
  const groups=colors.map((color,i)=>{
    const regionPaths=traced.regions.filter(r=>r.threadId===color).flatMap(r=>r.geometry.rings.map(ring=>ring.map(p=>({x:p.x+view.physicalWidthMm/2-framed.originX,y:p.y+view.physicalHeightMm/2-framed.originY}))));
    return `<g id="separation-${i+1}" data-ink="${escapeXML(color)}" fill="${validColor(color)}" fill-rule="evenodd">${regionPaths.map(path=>`<path d="${pathToSVG(path)}"/>`).join("")}</g>`;
  }).join("");
  const svg=svgEnvelope(framed.widthMM,framed.heightMM,groups);const blob=new Blob([svg],{type:"image/svg+xml"});
  const sha256=await hashBlob(blob);
  warnings.push(`Sized to inked artwork (${framed.widthMM.toFixed(1)} × ${framed.heightMM.toFixed(1)} mm), not the full print area.`);
  return{method,blob,fileName:`${clean}-screen-separations.svg`,mime:"image/svg+xml",previewUrl:URL.createObjectURL(blob),summary:`Layered screen-print SVG · ${colors.length} ink separation${colors.length===1?"":"s"} · ${framed.widthMM.toFixed(1)} × ${framed.heightMM.toFixed(1)} mm`,warnings,widthMM:framed.widthMM,heightMM:framed.heightMM,sha256,renderer:"server"};
}

async function cleanVinylPaths(ringsList:{x:number;y:number}[][][],view:DigitizerView):Promise<PolygonPaths>{
  // Each entry is one traced shape: rings[0]=exterior, rings[1+]=holes/cutouts.
  const polygons=ringsList
    .map(rings=>rings.map(ring=>ring.map(p=>({x:p.x+view.physicalWidthMm/2,y:p.y+view.physicalHeightMm/2}))))
    .filter(rings=>rings.length>0&&rings[0].length>=3);
  if(!polygons.length)throw new Error("No vinyl contours were produced.");

  // Keep each letter/logo as one path set (outer + holes). Flattening holes into the
  // union list used to treat cutouts as solid fills, so only outer outlines survived.
  const shapes:PolygonPaths[]=[];
  for(const rings of polygons){
    let shape:PolygonPaths=[orientRing(rings[0],true)];
    for(let i=1;i<rings.length;i++){
      if(rings[i].length<3)continue;
      const cut=await api.productionBoolean(shape,[orientRing(rings[i],false)],"difference");
      shape=cut.paths.length?cut.paths:shape;
    }
    if(shape.length)shapes.push(shape);
  }
  if(!shapes.length)throw new Error("No vinyl contours were produced.");

  let merged:PolygonPaths=shapes[0];
  for(let i=1;i<shapes.length;i++){
    const result=await api.productionBoolean(merged,shapes[i],"union");
    merged=result.paths.length?result.paths:merged.concat(shapes[i]);
  }
  const normalized=await api.productionOffset(merged,0,"round");
  return normalized.paths.length?normalized.paths:merged;
}

function orientRing(ring:{x:number;y:number}[],wantPositiveArea:boolean){
  let area=0;
  for(let i=0;i<ring.length;i++){
    const p=ring[i],q=ring[(i+1)%ring.length];
    area+=p.x*q.y-q.x*p.y;
  }
  const positive=area>=0;
  return positive===wantPositiveArea?ring:ring.slice().reverse();
}

export function framePathsToContent(paths:{x:number;y:number}[][],marginMM=CONTENT_MARGIN_MM){
  let minX=Infinity,minY=Infinity,maxX=-Infinity,maxY=-Infinity;
  for(const path of paths)for(const p of path){if(p.x<minX)minX=p.x;if(p.y<minY)minY=p.y;if(p.x>maxX)maxX=p.x;if(p.y>maxY)maxY=p.y}
  if(!Number.isFinite(minX)||!Number.isFinite(minY)||maxX<minX||maxY<minY)throw new Error("No artwork contours were produced to size the export.");
  const originX=minX-marginMM,originY=minY-marginMM;
  const widthMM=Math.max(0.1,maxX-minX+marginMM*2),heightMM=Math.max(0.1,maxY-minY+marginMM*2);
  return{originX,originY,widthMM,heightMM,paths:paths.map(path=>path.map(p=>({x:p.x-originX,y:p.y-originY})))};
}

function pathToSVG(path:{x:number;y:number}[]){return path.map((p,i)=>`${i?"L":"M"}${p.x.toFixed(3)} ${p.y.toFixed(3)}`).join(" ")+" Z"}
function svgEnvelope(widthMM:number,heightMM:number,body:string){return `<svg xmlns="http://www.w3.org/2000/svg" width="${widthMM}mm" height="${heightMM}mm" viewBox="0 0 ${widthMM} ${heightMM}"><metadata>PrintStudio production export; units=mm; sized to artwork content</metadata>${body}</svg>`}
function coversCanvas(elements:ExportElement[],view:DigitizerView){return elements.some(e=>e.x<=0&&e.y<=0&&e.x+e.w>=view.canvasWidth&&e.y+e.h>=view.canvasHeight)}
function layerName(e:ExportElement){return e.type==="text"?`Text “${e.value.slice(0,20)}”`:"Uploaded artwork"}
function safeName(name:string){return name.trim().replace(/[^a-z0-9_-]+/gi,"-").replace(/^-|-$/g,"")||"printstudio-design"}
function escapeXML(value:string){return value.replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&apos;"}[c]!))}
function validColor(value:string){return /^#[0-9a-f]{6}$/i.test(value)?value:"#000000"}
async function hashBlob(blob:Blob){const digest=await crypto.subtle.digest("SHA-256",await blob.arrayBuffer());return Array.from(new Uint8Array(digest),b=>b.toString(16).padStart(2,"0")).join("")}
