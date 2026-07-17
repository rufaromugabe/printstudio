export type CloudDesign<T> = { id: string; name: string; document: T; version: number; updatedAt: string };
export type ProductOption={value:string;label:string};
export type ProductView={id:string;label:string;canvasWidth:number;canvasHeight:number;physicalWidthMm:number;physicalHeightMm:number;safeMarginMm:number;bleedMm:number;mockup:{kind?:string;baseAssetId?:string|null;maskAssetId?:string|null;shadowAssetId?:string|null;highlightAssetId?:string|null}};
export type ProductTemplate={version:number;category:string;views:ProductView[];properties:{id:string;label:string;type:"select"|"text"|"number"|"boolean";required:boolean;options:ProductOption[]}[];colors:ProductOption[]};
export type Product = { id: string; name: string; methods: string[]; views: string[]; active: boolean;template:ProductTemplate };
export type Asset = { id:string; fileName:string; contentType:string; size:number; width:number; height:number; status:string; url:string };
export type EmbroideryPoint={x:number;y:number};
export type EmbroideryRegion={id:string;threadId:string;geometry:{rings:EmbroideryPoint[][]};kind:"running"|"tatami"|"satin";spacingMm?:number;stitchLengthMm?:number;angleDegrees?:number;edgeUnderlay?:boolean;centerUnderlay?:boolean;zigzagUnderlay?:boolean};
export type EmbroideryRequest={name:string;regions:EmbroideryRegion[];machine:{id:string;name:string;hoopWidthMm:number;hoopHeightMm:number;maxStitches:number;maxColors:number;minStitchMm:number;maxStitchMm:number;maxJumpMm:number}};
export type EmbroideryDiagnostic={severity:"error"|"warning";code:string;message:string;regionId:string};
export type EmbroideryCompilation={document:{sourceHash:string;compilerVersion:string;diagnostics:EmbroideryDiagnostic[];plan:{underlay:unknown[];stitches:unknown[]}[]};svg:string};
const base = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token=typeof window!=="undefined"?localStorage.getItem("printstudio-google-token"):null;
  const response = await fetch(`${base}${path}`, { ...init, headers: { "Content-Type": "application/json", ...(token?{Authorization:`Bearer ${token}`}:{}) ,...init?.headers } });
  if (!response.ok) { const body = await response.json().catch(() => ({})); throw new Error(body.message ?? `Request failed (${response.status})`); }
  return response.json();
}
async function requestBlob(path:string,body:Blob):Promise<Blob>{const token=typeof window!=="undefined"?localStorage.getItem("printstudio-google-token"):null;const response=await fetch(`${base}${path}`,{method:"POST",headers:{"Content-Type":body.type||"image/png",...(token?{Authorization:`Bearer ${token}`}:{})},body});if(!response.ok){const problem=await response.json().catch(()=>({}));throw new Error(problem.message??`Production request failed (${response.status})`)}return response.blob()}
export const api = {
  products: () => request<Product[]>("/v1/products"),
  designs: <T>() => request<CloudDesign<T>[]>("/v1/designs"),
  create: <T>(name: string, document: T) => request<CloudDesign<T>>("/v1/designs", { method: "POST", body: JSON.stringify({ name, document }) }),
  update: <T>(id: string, version:number, name: string, document: T) => request<CloudDesign<T>>(`/v1/designs/${id}`, { method: "PUT", body: JSON.stringify({ version, name, document }) }),
  share: (id: string) => request<{ token: string; expiresAt: string }>(`/v1/designs/${id}/shares`, { method: "POST" }),
  uploadAsset: async (file: File) => {
    const digest=await crypto.subtle.digest("SHA-256",await file.arrayBuffer());const sha256=Array.from(new Uint8Array(digest),b=>b.toString(16).padStart(2,"0")).join("");
    const ticket = await request<{assetId:string;uploadUrl:string}>("/v1/assets/uploads", { method:"POST", body:JSON.stringify({fileName:file.name,contentType:file.type,size:file.size,sha256}) });
    const uploaded = await fetch(ticket.uploadUrl,{method:"PUT",headers:{"Content-Type":file.type},body:file});
    if(!uploaded.ok) throw new Error("Object upload failed");
    return request<Asset>(`/v1/assets/${ticket.assetId}/complete`,{method:"POST"});
  },
  assetURL: (id:string) => request<{url:string;expiresIn:number}>(`/v1/assets/${id}/url`),
  compileEmbroidery:(input:EmbroideryRequest)=>request<EmbroideryCompilation>("/v1/embroidery/compile",{method:"POST",body:JSON.stringify(input)}),
  exportEmbroidery:async(input:EmbroideryRequest)=>{
    const token=typeof window!=="undefined"?localStorage.getItem("printstudio-google-token"):null;
    const response=await fetch(`${base}/v1/embroidery/export/dst`,{method:"POST",headers:{"Content-Type":"application/json",...(token?{Authorization:`Bearer ${token}`}:{})},body:JSON.stringify(input)});
    if(!response.ok){const body=await response.json().catch(()=>({}));throw new Error(body.message??`Export failed (${response.status})`)}
    return response.blob();
  },
  productionUnderbase:(artwork:Blob,spreadPixels:number,threshold=1)=>requestBlob(`/v1/production/dtf/underbase?spread=${spreadPixels}&threshold=${threshold}`,artwork),
  productionHalftone:(artwork:Blob,dpi=300,lpi=45,angle=22.5,gamma=1)=>requestBlob(`/v1/production/screen/halftone?dpi=${dpi}&lpi=${lpi}&angle=${angle}&gamma=${gamma}`,artwork),
  productionCMYK:(artwork:Blob)=>requestBlob("/v1/production/screen/cmyk",artwork),
  productionCapabilities:()=>request<{icc:boolean;vectorTrace:boolean;polygonBoolean:boolean;vipsPath:string;potracePath:string}>("/v1/production/capabilities"),
};
