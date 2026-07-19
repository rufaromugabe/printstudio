import { api, EmbroideryKind, EmbroideryPoint, EmbroideryRegion, OCRReport, VectorContourSet, VectorPrepMetadata, VectorSimilarityReport } from "./api";
import { nearestThread, ThreadBrand } from "./thread-charts";

export type DigitizerEmbroideryKind="auto"|EmbroideryKind;
export type DigitizerElement={id:string;type:"text"|"image";value:string;x:number;y:number;w:number;h:number;rotation:number;color:string;fontSize:number;fontFamily?:string;fontWeight?:number;fontStyle?:"normal"|"italic";letterSpacing?:number;strokeColor?:string;strokeWidth?:number;curveType?:"straight"|"circle";curveRadius?:number;curveSweep?:number;curveDirection?:"clockwise"|"counterclockwise";embroideryKind?:DigitizerEmbroideryKind;embroiderySpacing?:number;embroideryAngle?:number;embroideryUnderlay?:"auto"|"none"|"edge"|"center-zigzag";embroideryStitchLength?:number;embroideryFoamHeightMm?:2|3;assetId?:string};
export type DigitizerView={canvasWidth:number;canvasHeight:number;physicalWidthMm:number;physicalHeightMm:number};
export type Digitization={regions:EmbroideryRegion[];fallbacks:string[];threadLabels:Record<string,string>;vectorDiagnostics?:{code:string;message:string;severity:string}[];vectorReports?:VectorPrepMetadata[];vectorSimilarities?:VectorSimilarityReport[];ocrReports?:{elementId:string;report:OCRReport}[]};

const MAX_IMAGE_THREADS=8;
const ALPHA_CUTOFF=32;

export type DigitizeOptions={
  threadBrand?:ThreadBrand;
  mode?:"color"|"silhouette";
  method?:"vinyl"|"embroidery"|"screen"|string;
  /** Optional ML prep before Potrace. Image layers always use server vectorize. */
  mlPrep?:boolean;
};

export async function digitizeElements(elements:DigitizerElement[],view:DigitizerView,options?:DigitizeOptions):Promise<Digitization>{
  const regions:EmbroideryRegion[]=[],failures:string[]=[],threadLabels:Record<string,string>={},brand=options?.threadBrand??"none",mode=options?.mode??"color";
  const method=(options?.method??"embroidery") as "vinyl"|"embroidery"|"screen";
  const mlPrep=options?.mlPrep??true;
  const vectorDiagnostics:{code:string;message:string;severity:string}[]=[];
  const vectorReports:VectorPrepMetadata[]=[];
  const vectorSimilarities:VectorSimilarityReport[]=[];
  const ocrReports:{elementId:string;report:OCRReport}[]=[];
  for(const element of elements){
    try{
      const layers=element.type==="image"
        ? await extractImageLayersServer(element,view,mode,method,mlPrep,vectorDiagnostics,vectorReports,vectorSimilarities,ocrReports)
        : await extractTextLayers(element);
      if(!layers.length){failures.push(element.id);continue}
      for(const [layerIndex,layer] of layers.entries()){
        const matched=nearestThread(layer.threadId,brand);
        threadLabels[matched.hex]=matched.label;
        const polygons=groupRings(layer.rings);
        const spacingMm=element.embroiderySpacing??.45;
        let regionIndex=0;
        for(const polygon of polygons){
          const autoSatin=element.type==="text"&&polygons.length===1&&polygon.length<=2;
          const kind=element.embroideryKind&&element.embroideryKind!=="auto"?element.embroideryKind:(autoSatin?"satin":"tatami");
          const underlay=element.embroideryUnderlay??"auto";
          const satin=kind==="satin";
          const puff=kind==="puff";
          const ownUnderlay=puff||kind==="applique"||kind==="sequin";
          const columnLike=satin||puff||kind==="applique";
          const rings=layer.units==="mm"
            ? polygon
            : polygon.map(r=>r.map(p=>toPhysical(p,element,view)));
          // Skip islands thinner than one tatami row — they cannot produce fill stitches.
          // Measure the exterior only; holes are allowed to be finer.
          const minFeature=polygonMinFeatureMm(rings[0]?[rings[0]]:[]);
          if(puff&&minFeature>0&&minFeature<5){
            continue;
          }
          if(!columnLike&&minFeature>0&&minFeature<Math.max(0.8,spacingMm*1.5)){
            continue;
          }
          const region:EmbroideryRegion={
            id:`${element.id}-${layerIndex}-${regionIndex++}`,
            threadId:matched.hex,
            geometry:{rings},
            kind,
            stitchLengthMm:element.embroideryStitchLength??3,
            angleDegrees:element.embroideryAngle??0,
            edgeUnderlay:ownUnderlay?false:underlay==="edge"||(underlay==="auto"&&!satin),
            centerUnderlay:ownUnderlay?false:underlay==="center-zigzag"||(underlay==="auto"&&satin),
            zigzagUnderlay:ownUnderlay?false:underlay==="center-zigzag"||(underlay==="auto"&&satin),
          };
          if(puff){
            region.foamHeightMm=element.embroideryFoamHeightMm===2?2:3;
            // Omit spacing so the compiler applies foam-driven cover density.
          }else if(kind==="applique"){
            region.widthMm=2;
            region.spacingMm=spacingMm;
          }else if(kind==="sequin"){
            region.spacingMm=Math.max(spacingMm,4);
          }else{
            region.spacingMm=spacingMm;
          }
          regions.push(region);
        }
      }
    }catch(error){
      if(element.type==="image"){
        throw error instanceof Error?error:new Error("Server vectorize failed for an image layer");
      }
      failures.push(element.id);
    }
  }
  if(failures.length)throw new Error(`${failures.length} layer(s) could not be traced into production contours. Re-upload artwork with CORS-readable pixels or convert to editable text/shapes.`);
  if(!regions.length)throw new Error("No production contours were produced from the design.");
  return{regions,fallbacks:[],threadLabels,vectorDiagnostics,vectorReports,vectorSimilarities,ocrReports};
}

type ColorLayer={threadId:string;rings:EmbroideryPoint[][];units?:"px"|"mm"};

async function extractImageLayersServer(
  element:DigitizerElement,
  view:DigitizerView,
  mode:"color"|"silhouette",
  method:"vinyl"|"embroidery"|"screen",
  mlPrep:boolean,
  diagnostics:{code:string;message:string;severity:string}[],
  reports:VectorPrepMetadata[],
  similarities:VectorSimilarityReport[],
  ocrReports:{elementId:string;report:OCRReport}[],
):Promise<ColorLayer[]>{
  const placement={
    canvasWidth:view.canvasWidth,
    canvasHeight:view.canvasHeight,
    physicalWidthMm:view.physicalWidthMm,
    physicalHeightMm:view.physicalHeightMm,
    x:element.x,y:element.y,w:element.w,h:element.h,rotation:element.rotation,
  };
  if(mode==="silhouette"||element.color){
    const blob=await renderElementPNG(element);
    const contours=await api.productionVectorize(blob,{method,mlPrep,placement,includeProof:true});
    contours.ocr=await refineOCRFontMatch(element,contours.ocr);
    pushVectorResult(diagnostics,reports,similarities,ocrReports,element.id,contours);
    return[{threadId:element.color||"#222222",rings:contours.rings,units:"mm"}];
  }
  const blob=await renderElementPNG(element);
  const separated=await api.productionVectorizeColor(blob,{method,mlPrep,placement,maxColors:MAX_IMAGE_THREADS,includeProof:true});
  separated.ocr=await refineOCRFontMatch(element,separated.ocr);
  for(const diagnostic of separated.diagnostics??[])diagnostics.push(diagnostic);
  if(separated.ocr?.attempted&&!ocrReports.some(item=>item.elementId===element.id&&item.report.text===separated.ocr.text))ocrReports.push({elementId:element.id,report:separated.ocr});
  for(const layer of separated.layers)pushVectorResult(diagnostics,reports,similarities,ocrReports,element.id,layer.contours);
  return separated.layers.map(layer=>({threadId:layer.color,rings:layer.contours.rings,units:"mm"}));
}

function pushVectorResult(out:{code:string;message:string;severity:string}[],reports:VectorPrepMetadata[],similarities:VectorSimilarityReport[],ocrReports:{elementId:string;report:OCRReport}[],elementId:string,contours:VectorContourSet){
  for(const d of contours.diagnostics??[])out.push({code:d.code,message:d.message,severity:d.severity});
  if(contours.prep&&!reports.some(report=>report.profile===contours.prep.profile&&report.inputWidthPx===contours.prep.inputWidthPx&&report.inputHeightPx===contours.prep.inputHeightPx))reports.push(contours.prep);
  if(contours.similarity&&!similarities.some(report=>report.proofPngBase64===contours.similarity.proofPngBase64&&report.score===contours.similarity.score))similarities.push(contours.similarity);
  if(contours.ocr?.attempted&&!ocrReports.some(item=>item.elementId===elementId&&item.report.text===contours.ocr.text&&item.report.confidence===contours.ocr.confidence))ocrReports.push({elementId,report:contours.ocr});
}

/** Editable text stays on the glyph/canvas tracer (not a production fallback for images). */
async function extractTextLayers(element:DigitizerElement):Promise<ColorLayer[]>{
  const {width,height,data,scale}=await rasterizeElement(element);
  const mask=new Uint8Array(width*height);
  for(let i=0;i<mask.length;i++)mask[i]=data[i*4+3]>=ALPHA_CUTOFF?1:0;
  const rings=traceMask(mask,width,height).map(r=>simplify(r.map(p=>({x:p.x/scale,y:p.y/scale})),.35));
  if(!rings.length)return[];
  return[{threadId:element.color||"#222222",rings}];
}

async function rasterizeElement(element:DigitizerElement){
  const scale=Math.min(4,Math.max(1,800/Math.max(element.w,element.h)));
  const width=Math.max(2,Math.ceil(element.w*scale));
  const height=Math.max(2,Math.ceil(element.h*scale));
  const canvas=document.createElement("canvas");
  canvas.width=width;canvas.height=height;
  const ctx=canvas.getContext("2d",{willReadFrequently:true});
  if(!ctx)throw new Error("canvas unavailable");
  ctx.scale(scale,scale);
  if(element.type==="image"){
    const image=await loadImage(element.value);
    ctx.drawImage(image,0,0,element.w,element.h);
  }else{
    renderText(ctx,element);
  }
  return{width,height,scale,data:ctx.getImageData(0,0,width,height).data,canvas};
}

async function renderElementPNG(element:DigitizerElement):Promise<Blob>{
  const {canvas}=await rasterizeElement(element);
  const blob=await new Promise<Blob|null>(resolve=>canvas.toBlob(resolve,"image/png"));
  if(!blob)throw new Error("could not encode layer PNG for vectorize");
  return blob;
}

async function refineOCRFontMatch(element:DigitizerElement,report:OCRReport):Promise<OCRReport>{
  if(!report?.attempted||!report.text?.trim())return report;
  const {width,height,data}=await rasterizeElement(element);
  const source=foregroundMask(data,width,height);
  const bounds=maskBounds(source,width,height);
  if(!bounds)return report;
  const families=["Arial","Georgia","Verdana","Trebuchet MS","Courier New","Impact","Times New Roman"];
  const candidates:{family:string;confidence:number;reason:string;weight:number}[]=[];
  for(const family of families)for(const weight of [400,700,900]){
    const canvas=document.createElement("canvas");canvas.width=width;canvas.height=height;
    const ctx=canvas.getContext("2d",{willReadFrequently:true});if(!ctx)continue;
    ctx.fillStyle="#000";ctx.textAlign="center";ctx.textBaseline="middle";
    const lines=report.text.split("\n"),lineBox=Math.max(1,bounds.height/lines.length);
    let fontSize=lineBox*.9;
    ctx.font=`${weight} ${fontSize}px ${family}`;
    const widest=Math.max(1,...lines.map(line=>ctx.measureText(line).width));
    fontSize*=Math.min(1,bounds.width/widest);
    ctx.font=`${weight} ${Math.max(2,fontSize)}px ${family}`;
    lines.forEach((line,index)=>ctx.fillText(line,bounds.x+bounds.width/2,bounds.y+lineBox*(index+.5)));
    const candidateData=ctx.getImageData(0,0,width,height).data;
    const candidate=new Uint8Array(width*height);
    for(let i=0;i<candidate.length;i++)candidate[i]=candidateData[i*4+3]>=ALPHA_CUTOFF?1:0;
    const score=maskIoU(source,candidate);
    candidates.push({family,weight,confidence:Number(score.toFixed(3)),reason:`measured raster-shape match at weight ${weight}`});
  }
  candidates.sort((a,b)=>b.confidence-a.confidence);
  const best=candidates[0];
  return{...report,fontCandidates:candidates.slice(0,5),editableRebuildRecommended:report.confidence>=88&&Boolean(best&&best.confidence>=.42)};
}

function foregroundMask(data:Uint8ClampedArray,width:number,height:number){
  const mask=new Uint8Array(width*height),hasTransparency=(()=>{for(let i=3;i<data.length;i+=4)if(data[i]<245)return true;return false})();
  if(hasTransparency){for(let i=0;i<mask.length;i++)mask[i]=data[i*4+3]>=ALPHA_CUTOFF?1:0;return mask}
  const corners=[[0,0],[width-1,0],[0,height-1],[width-1,height-1]],background={r:0,g:0,b:0};
  for(const [x,y] of corners){const i=(y*width+x)*4;background.r+=data[i];background.g+=data[i+1];background.b+=data[i+2]}
  background.r/=4;background.g/=4;background.b/=4;
  for(let i=0;i<mask.length;i++){const o=i*4,dr=data[o]-background.r,dg=data[o+1]-background.g,db=data[o+2]-background.b;mask[i]=Math.sqrt(dr*dr+dg*dg+db*db)>=24?1:0}
  return mask;
}

function maskBounds(mask:Uint8Array,width:number,height:number){
  let minX=width,minY=height,maxX=-1,maxY=-1;
  for(let y=0;y<height;y++)for(let x=0;x<width;x++)if(mask[y*width+x]){minX=Math.min(minX,x);minY=Math.min(minY,y);maxX=Math.max(maxX,x);maxY=Math.max(maxY,y)}
  return maxX<minX?null:{x:minX,y:minY,width:maxX-minX+1,height:maxY-minY+1};
}

function maskIoU(a:Uint8Array,b:Uint8Array){let intersection=0,union=0;for(let i=0;i<a.length;i++){if(a[i]&&b[i])intersection++;if(a[i]||b[i])union++}return union?intersection/union:0}

function renderText(ctx:CanvasRenderingContext2D,e:DigitizerElement){
  ctx.fillStyle=e.color||"#000";
  ctx.strokeStyle=e.strokeColor??"transparent";
  ctx.lineWidth=(e.strokeWidth??0)*2;
  ctx.font=`${e.fontStyle??"normal"} ${e.fontWeight??400} ${e.fontSize}px ${e.fontFamily??"Arial"}`;
  ctx.textAlign="center";
  ctx.textBaseline="middle";
  const paint=(text:string,x:number,y:number)=>{if(ctx.lineWidth>0)ctx.strokeText(text,x,y);ctx.fillText(text,x,y)};
  if(e.curveType!=="circle"){
    const lines=e.value.split("\n"),line=e.fontSize*1.1;
    lines.forEach((text,i)=>paint(text,e.w/2,e.h/2+(i-(lines.length-1)/2)*line));
    return;
  }
  const chars=[...e.value],radius=Math.max(12,Math.min(e.curveRadius??85,Math.min(e.w,e.h)/2-2)),sweep=Math.max(30,Math.min(360,e.curveSweep??240))*Math.PI/180,sign=e.curveDirection==="counterclockwise"?-1:1;
  chars.forEach((char,i)=>{
    const t=chars.length===1?.5:i/(chars.length-1),angle=-Math.PI/2+sign*(t-.5)*sweep;
    ctx.save();
    ctx.translate(e.w/2+Math.cos(angle)*radius,e.h/2+Math.sin(angle)*radius);
    ctx.rotate(angle+sign*Math.PI/2);
    paint(char,0,0);
    ctx.restore();
  });
}

function loadImage(src:string){
  return new Promise<HTMLImageElement>((resolve,reject)=>{
    const image=new Image();
    image.crossOrigin="anonymous";
    image.onload=()=>resolve(image);
    image.onerror=()=>reject(new Error("image decode failed"));
    image.src=src;
  });
}

type Edge={a:EmbroideryPoint;b:EmbroideryPoint};
function traceMask(mask:Uint8Array,w:number,h:number){
  const edges:Edge[]=[],on=(x:number,y:number)=>x>=0&&y>=0&&x<w&&y<h&&mask[y*w+x]===1;
  for(let y=0;y<h;y++)for(let x=0;x<w;x++)if(on(x,y)){
    if(!on(x,y-1))edges.push({a:{x,y},b:{x:x+1,y}});
    if(!on(x+1,y))edges.push({a:{x:x+1,y},b:{x:x+1,y:y+1}});
    if(!on(x,y+1))edges.push({a:{x:x+1,y:y+1},b:{x,y:y+1}});
    if(!on(x-1,y))edges.push({a:{x,y:y+1},b:{x,y}});
  }
  const next=new Map<string,Edge[]>();
  for(const e of edges){const k=key(e.a);next.set(k,[...(next.get(k)??[]),e])}
  const used=new Set<Edge>(),rings:EmbroideryPoint[][]=[];
  for(const start of edges){
    if(used.has(start))continue;
    const ring:EmbroideryPoint[]=[];
    let edge:Edge|undefined=start;
    while(edge&&!used.has(edge)){
      used.add(edge);
      ring.push(edge.a);
      const candidates:Edge[]=next.get(key(edge.b))??[];
      edge=candidates.find((candidate:Edge)=>!used.has(candidate));
    }
    if(ring.length>=4)rings.push(ring);
  }
  return rings;
}
const key=(p:EmbroideryPoint)=>`${p.x},${p.y}`;
function signedArea(r:EmbroideryPoint[]){let a=0;for(let i=0;i<r.length;i++){const p=r[i],q=r[(i+1)%r.length];a+=p.x*q.y-q.x*p.y}return a/2}
function pointInRing(p:EmbroideryPoint,r:EmbroideryPoint[]){let inside=false;for(let i=0,j=r.length-1;i<r.length;j=i++){const a=r[i],b=r[j];if((a.y>p.y)!==(b.y>p.y)&&p.x<(b.x-a.x)*(p.y-a.y)/(b.y-a.y)+a.x)inside=!inside}return inside}
function groupRings(rings:EmbroideryPoint[][]){
  const sorted=rings.filter(r=>Math.abs(signedArea(r))>.5).sort((a,b)=>Math.abs(signedArea(b))-Math.abs(signedArea(a)));
  const groups:EmbroideryPoint[][][]=[];
  for(const ring of sorted){
    let container=-1;
    for(let i=0;i<groups.length;i++)if(pointInRing(ring[0],groups[i][0])){container=i;break}
    if(container>=0)groups[container].push(ring);else groups.push([ring]);
  }
  return groups;
}
function polygonMinFeatureMm(rings:EmbroideryPoint[][]){
  let min=Infinity;
  for(const ring of rings){
    if(ring.length<3)continue;
    let minX=Infinity,minY=Infinity,maxX=-Infinity,maxY=-Infinity;
    for(const p of ring){
      minX=Math.min(minX,p.x);minY=Math.min(minY,p.y);
      maxX=Math.max(maxX,p.x);maxY=Math.max(maxY,p.y);
    }
    const feat=Math.min(maxX-minX,maxY-minY);
    if(feat>0)min=Math.min(min,feat);
  }
  return Number.isFinite(min)?min:0;
}
function simplify(points:EmbroideryPoint[],tolerance:number){
  if(points.length<4)return points;
  const out:EmbroideryPoint[]=[];
  for(let i=0;i<points.length;i++){
    const a=points[(i+points.length-1)%points.length],b=points[i],c=points[(i+1)%points.length];
    const cross=Math.abs((b.x-a.x)*(c.y-b.y)-(b.y-a.y)*(c.x-b.x));
    if(cross>tolerance)out.push(b);
  }
  return out.length>=3?out:points;
}
function toPhysical(p:EmbroideryPoint,e:DigitizerElement,v:DigitizerView){
  const x=e.x+p.x,y=e.y+p.y,cx=e.x+e.w/2,cy=e.y+e.h/2,a=e.rotation*Math.PI/180,c=Math.cos(a),s=Math.sin(a);
  const rx=cx+(x-cx)*c-(y-cy)*s,ry=cy+(x-cx)*s+(y-cy)*c;
  return{x:rx/v.canvasWidth*v.physicalWidthMm-v.physicalWidthMm/2,y:ry/v.canvasHeight*v.physicalHeightMm-v.physicalHeightMm/2};
}
