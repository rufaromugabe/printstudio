import { api, EmbroideryPoint, EmbroideryRegion, VectorContourSet } from "./api";
import { nearestThread, ThreadBrand } from "./thread-charts";

export type DigitizerElement={id:string;type:"text"|"image";value:string;x:number;y:number;w:number;h:number;rotation:number;color:string;fontSize:number;fontFamily?:string;fontWeight?:number;fontStyle?:"normal"|"italic";letterSpacing?:number;strokeColor?:string;strokeWidth?:number;curveType?:"straight"|"circle";curveRadius?:number;curveSweep?:number;curveDirection?:"clockwise"|"counterclockwise";embroideryKind?:"auto"|"running"|"tatami"|"satin";embroiderySpacing?:number;embroideryAngle?:number;embroideryUnderlay?:"auto"|"none"|"edge"|"center-zigzag";embroideryStitchLength?:number;assetId?:string};
export type DigitizerView={canvasWidth:number;canvasHeight:number;physicalWidthMm:number;physicalHeightMm:number};
export type Digitization={regions:EmbroideryRegion[];fallbacks:string[];threadLabels:Record<string,string>;vectorDiagnostics?:{code:string;message:string;severity:string}[]};

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
  for(const element of elements){
    try{
      const layers=element.type==="image"
        ? await extractImageLayersServer(element,view,mode,method,mlPrep,vectorDiagnostics)
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
          const rings=layer.units==="mm"
            ? polygon
            : polygon.map(r=>r.map(p=>toPhysical(p,element,view)));
          // Skip islands thinner than one tatami row — they cannot produce fill stitches.
          // Measure the exterior only; holes are allowed to be finer.
          const minFeature=polygonMinFeatureMm(rings[0]?[rings[0]]:[]);
          if(!satin&&minFeature>0&&minFeature<Math.max(0.8,spacingMm*1.5)){
            continue;
          }
          regions.push({
            id:`${element.id}-${layerIndex}-${regionIndex++}`,
            threadId:matched.hex,
            geometry:{rings},
            kind,
            spacingMm,
            stitchLengthMm:element.embroideryStitchLength??3,
            angleDegrees:element.embroideryAngle??0,
            edgeUnderlay:underlay==="edge"||(underlay==="auto"&&!satin),
            centerUnderlay:underlay==="center-zigzag"||(underlay==="auto"&&satin),
            zigzagUnderlay:underlay==="center-zigzag"||(underlay==="auto"&&satin),
          });
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
  return{regions,fallbacks:[],threadLabels,vectorDiagnostics};
}

type ColorLayer={threadId:string;rings:EmbroideryPoint[][];units?:"px"|"mm"};

async function extractImageLayersServer(
  element:DigitizerElement,
  view:DigitizerView,
  mode:"color"|"silhouette",
  method:"vinyl"|"embroidery"|"screen",
  mlPrep:boolean,
  diagnostics:{code:string;message:string;severity:string}[],
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
    const contours=await api.productionVectorize(blob,{method,mlPrep,placement});
    pushDiagnostics(diagnostics,contours);
    return[{threadId:element.color||"#222222",rings:contours.rings,units:"mm"}];
  }
  const {width,height,data,scale}=await rasterizeElement(element);
  const palette=buildPalette(data,width,height,MAX_IMAGE_THREADS);
  if(!palette.length)return[];
  const labels=assignPixels(data,width,height,palette);
  const layers:ColorLayer[]=[];
  for(let c=0;c<palette.length;c++){
    const mask=new Uint8Array(width*height);
    let count=0;
    for(let i=0;i<mask.length;i++)if(labels[i]===c){mask[i]=1;count++}
    if(count<8)continue;
    const blob=await maskToPNG(mask,width,height);
    // Rings come back in mask pixel space; map into element-local px then physical mm client-side.
    const contours=await api.productionVectorize(blob,{method,mlPrep});
    pushDiagnostics(diagnostics,contours);
    const rings=contours.rings.map(r=>r.map(p=>({x:p.x/scale,y:p.y/scale})));
    layers.push({threadId:rgbHex(palette[c]),rings,units:"px"});
  }
  return layers;
}

function pushDiagnostics(out:{code:string;message:string;severity:string}[],contours:VectorContourSet){
  for(const d of contours.diagnostics??[])out.push({code:d.code,message:d.message,severity:d.severity});
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

async function maskToPNG(mask:Uint8Array,w:number,h:number):Promise<Blob>{
  const canvas=document.createElement("canvas");
  canvas.width=w;canvas.height=h;
  const ctx=canvas.getContext("2d");
  if(!ctx)throw new Error("canvas unavailable");
  const image=ctx.createImageData(w,h);
  for(let i=0;i<mask.length;i++){
    const on=mask[i]===1;
    const o=i*4;
    image.data[o]=0;image.data[o+1]=0;image.data[o+2]=0;image.data[o+3]=on?255:0;
  }
  ctx.putImageData(image,0,0);
  const blob=await new Promise<Blob|null>(resolve=>canvas.toBlob(resolve,"image/png"));
  if(!blob)throw new Error("could not encode mask PNG for vectorize");
  return blob;
}

type RGB={r:number;g:number;b:number};

function buildPalette(data:Uint8ClampedArray,w:number,h:number,maxColors:number):RGB[]{
  // Popularity quantize: bucket RGB, keep the largest opaque buckets, then merge leftovers.
  const buckets=new Map<number, {rgb:RGB;count:number}>();
  for(let y=0;y<h;y++)for(let x=0;x<w;x++){
    const i=(y*w+x)*4;
    if(data[i+3]<ALPHA_CUTOFF)continue;
    const r=data[i],g=data[i+1],b=data[i+2];
    // Skip near-white fluff often left after soft edges.
    if(r>245&&g>245&&b>245)continue;
    const key=((r>>4)<<8)|((g>>4)<<4)|(b>>4);
    const existing=buckets.get(key);
    if(existing){
      existing.count++;
      existing.rgb.r+=r;existing.rgb.g+=g;existing.rgb.b+=b;
    }else{
      buckets.set(key,{rgb:{r,g,b},count:1});
    }
  }
  const ranked=[...buckets.values()].map(item=>({
    rgb:{r:item.rgb.r/item.count,g:item.rgb.g/item.count,b:item.rgb.b/item.count},
    count:item.count,
  })).sort((a,b)=>b.count-a.count);
  if(!ranked.length)return[];
  const palette=ranked.slice(0,maxColors).map(item=>({r:Math.round(item.rgb.r),g:Math.round(item.rgb.g),b:Math.round(item.rgb.b)}));
  return mergeCloseColors(palette,28);
}

function mergeCloseColors(palette:RGB[],threshold:number):RGB[]{
  const out:RGB[]=[];
  for(const color of palette){
    const near=out.find(existing=>colorDistance(existing,color)<threshold);
    if(!near)out.push(color);
  }
  return out.length?out:palette.slice(0,1);
}

function assignPixels(data:Uint8ClampedArray,w:number,h:number,palette:RGB[]):Int8Array{
  const labels=new Int8Array(w*h).fill(-1);
  for(let y=0;y<h;y++)for(let x=0;x<w;x++){
    const i=(y*w+x)*4;
    if(data[i+3]<ALPHA_CUTOFF)continue;
    const pixel={r:data[i],g:data[i+1],b:data[i+2]};
    if(pixel.r>245&&pixel.g>245&&pixel.b>245)continue;
    let best=0,bestDist=Infinity;
    for(let c=0;c<palette.length;c++){
      const dist=colorDistance(pixel,palette[c]);
      if(dist<bestDist){bestDist=dist;best=c}
    }
    labels[y*w+x]=best;
  }
  return labels;
}

function colorDistance(a:RGB,b:RGB){
  const dr=a.r-b.r,dg=a.g-b.g,db=a.b-b.b;
  return Math.sqrt(dr*dr+dg*dg+db*db);
}

function rgbHex(c:RGB){
  return `#${[c.r,c.g,c.b].map(v=>Math.max(0,Math.min(255,v)).toString(16).padStart(2,"0")).join("")}`;
}

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
