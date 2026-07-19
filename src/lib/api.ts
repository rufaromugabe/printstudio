import { authHeaders, handleUnauthorized } from "@/lib/auth";

export type CloudDesign<T> = { id: string; name: string; document: T; version: number; updatedAt: string };
export type ProductOption={value:string;label:string};
export type ProductView={id:string;label:string;canvasWidth:number;canvasHeight:number;physicalWidthMm:number;physicalHeightMm:number;safeMarginMm:number;bleedMm:number;mockup:{kind?:string;baseAssetId?:string|null;maskAssetId?:string|null;shadowAssetId?:string|null;highlightAssetId?:string|null}};
export type ProductTemplate={version:number;category:string;views:ProductView[];properties:{id:string;label:string;type:"select"|"text"|"number"|"boolean";required:boolean;options:ProductOption[]}[];colors:ProductOption[]};
export type Product = { id: string; name: string; methods: string[]; views: string[]; active: boolean;template:ProductTemplate };
export type Asset = { id:string; fileName:string; contentType:string; size:number; width:number; height:number; status:string; url:string };
export type AuditEvent = { action: string; resourceId: string; actorId: string; createdAt: string };
export type ICCProfile = { id: string; label: string; fileName: string; sha256: string; size: number; version: number; createdAt: string; description?: string };
export type ProductionMetrics = {
  counters: Record<string, number>;
  capabilities: {
    icc?: boolean;
    vectorTrace?: boolean;
    polygonBoolean?: boolean;
    maxRenderPixels?: number;
    vipsPath?: string;
    potracePath?: string;
    iccProfiles?: boolean;
    productionReady?: boolean;
  };
  requireNatives: boolean;
  requireIcc: boolean;
};
export type EmbroideryPoint={x:number;y:number};
export type EmbroideryKind="running"|"tatami"|"satin"|"puff"|"bean"|"applique"|"motif"|"contour"|"estitch"|"cross"|"sequin"|"cord"|"chenille";
export type EmbroideryRegion={id:string;threadId:string;geometry:{rings:EmbroideryPoint[][]};kind:EmbroideryKind;spacingMm?:number;stitchLengthMm?:number;angleDegrees?:number;widthMm?:number;foamHeightMm?:number;edgeUnderlay?:boolean;centerUnderlay?:boolean;zigzagUnderlay?:boolean};
export type EmbroideryFabricClass="woven"|"tshirt"|"pique"|"fleece"|"performance-knit";
export type EmbroideryRequest={name:string;fabricClass?:EmbroideryFabricClass;regions:EmbroideryRegion[];machine:{id:string;name:string;hoopWidthMm:number;hoopHeightMm:number;maxStitches:number;maxColors:number;minStitchMm:number;maxStitchMm:number;maxJumpMm:number}};
export type EmbroideryDiagnostic={severity:"error"|"warning";code:string;message:string;regionId:string};
export type EmbroideryReview={score:number;decision:"auto"|"semi-auto"|"human"|"blocked";summary:string;factors:{code:string;label:string;points:number}[];fabric:{class:string;label:string;densityMm:number;pullCompensationMm:number;notes:string};hardStops?:string[]};
export type EmbroideryCompilation={document:{sourceHash:string;compilerVersion:string;diagnostics:EmbroideryDiagnostic[]|null;fabric?:EmbroideryReview["fabric"];review?:EmbroideryReview;plan:{underlay:unknown[]|null;stitches:unknown[]|null;bounds?:{minX:number;minY:number;maxX:number;maxY:number}}[]|null};svg:string};
export type VinylMaterialClass="htv-smooth"|"htv-flock"|"htv-glitter"|"adhesive-permanent"|"adhesive-removable"|"adhesive-glitter";
export type VinylDiagnostic={severity:"error"|"warning";code:string;message:string;pathId?:string};
export type VinylMaterialProfile={class:VinylMaterialClass|string;label:string;family:string;mirrorDefault:boolean;warnFeatureMm:number;rejectFeatureMm:number;notes:string};
export type VinylReview={score:number;decision:"auto"|"semi-auto"|"human"|"blocked";summary:string;factors:{code:string;label:string;points:number}[];material:VinylMaterialProfile;hardStops?:string[]};
export type VinylReviewResult={profile:VinylMaterialProfile;diagnostics:VinylDiagnostic[];review:VinylReview;mirrorRecommended:boolean};
export type PolygonPoint={x:number;y:number};
export type PolygonPaths=PolygonPoint[][];
export type VectorPoint={x:number;y:number};
export type VectorDiagnostic={severity:"error"|"warning";code:string;message:string};
export type VectorContourSet={
  rings:VectorPoint[][];
  sourceHash:string;
  tracer:"potrace"|"ml-assisted"|string;
  widthPx:number;
  heightPx:number;
  pathCount:number;
  minFeatureMm:number;
  units:"px"|"mm"|string;
  diagnostics?:VectorDiagnostic[];
};
export type VectorizePlacement={
  canvasWidth:number;
  canvasHeight:number;
  physicalWidthMm:number;
  physicalHeightMm:number;
  x:number;y:number;w:number;h:number;
  rotation:number;
};
const base = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${base}${path}`, {
    ...init,
    credentials: "include",
    headers: { "Content-Type": "application/json", ...authHeaders(init?.headers) },
  });
  if (response.status === 401) {
    handleUnauthorized();
    const body = await response.json().catch(() => ({}));
    throw new Error(body.message ?? "Session expired — sign in again");
  }
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new Error(body.message ?? `Request failed (${response.status})`);
  }
  return response.json();
}
async function requestBlob(path: string, body: Blob): Promise<Blob> {
  const response = await fetch(`${base}${path}`, {
    method: "POST",
    credentials: "include",
    headers: { ...authHeaders({ "Content-Type": body.type || "image/png" }) },
    body,
  });
  if (response.status === 401) {
    handleUnauthorized();
    const problem = await response.json().catch(() => ({}));
    throw new Error(problem.message ?? "Session expired — sign in again");
  }
  if (!response.ok) {
    const problem = await response.json().catch(() => ({}));
    throw new Error(problem.message ?? `Production request failed (${response.status})`);
  }
  return response.blob();
}
export const api = {
  products: () => request<Product[]>("/v1/products"),
  upsertProduct: (product: Product) => request<Product>("/v1/products", { method: "POST", body: JSON.stringify(product) }),
  audit: () => request<AuditEvent[]>("/v1/audit"),
  productionMetrics: () => request<ProductionMetrics>("/v1/production/metrics"),
  iccProfiles: () => request<{ profiles: ICCProfile[] }>("/v1/production/icc/profiles"),
  uploadIccProfile: async (file: Blob, options: { id: string; label?: string; description?: string }) => {
    const query = new URLSearchParams({ id: options.id });
    if (options.label) query.set("label", options.label);
    if (options.description) query.set("description", options.description);
    const response = await fetch(`${base}/v1/production/icc/profiles?${query}`, {
      method: "POST",
      credentials: "include",
      headers: { ...authHeaders({ "Content-Type": file.type || "application/octet-stream" }) },
      body: file,
    });
    if (response.status === 401) {
      handleUnauthorized();
      const body = await response.json().catch(() => ({}));
      throw new Error(body.message ?? "Session expired — sign in again");
    }
    if (!response.ok) {
      const body = await response.json().catch(() => ({}));
      throw new Error(body.message ?? `ICC upload failed (${response.status})`);
    }
    return response.json() as Promise<ICCProfile>;
  },
  assets: () => request<Asset[]>("/v1/assets"),
  designs: <T>() => request<CloudDesign<T>[]>("/v1/designs"),
  design: <T>(id: string) => request<CloudDesign<T>>(`/v1/designs/${id}`),
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
  vinylReview:(input:{materialClass:VinylMaterialClass|string;paths:PolygonPaths;mirrored?:boolean})=>request<VinylReviewResult>("/v1/vinyl/review",{method:"POST",body:JSON.stringify(input)}),
  exportEmbroidery:async(input:EmbroideryRequest)=>{
    const response=await fetch(`${base}/v1/embroidery/export/dst`,{method:"POST",credentials:"include",headers:{"Content-Type":"application/json",...authHeaders()},body:JSON.stringify(input)});
    if(response.status===401){handleUnauthorized();const body=await response.json().catch(()=>({}));throw new Error(body.message??"Session expired — sign in again")}
    if(!response.ok){const body=await response.json().catch(()=>({}));throw new Error(body.message??`Export failed (${response.status})`)}
    return response.blob();
  },
  productionUnderbase:(artwork:Blob,spreadPixels:number,threshold=1)=>requestBlob(`/v1/production/dtf/underbase?spread=${spreadPixels}&threshold=${threshold}`,artwork),
  productionCMYK:(artwork:Blob)=>requestBlob("/v1/production/screen/cmyk",artwork),
  productionGangRender:(artwork:Blob,options:{name:string;sourceWidthMm:number;sourceHeightMm:number;sheetWidthMm:number;sheetHeightMm:number;copies?:number;fillSheet?:boolean;dpi?:number;gapMm?:number;marginMm?:number;allowRotate?:boolean})=>{const query=new URLSearchParams({name:options.name,sourceWidthMm:String(options.sourceWidthMm),sourceHeightMm:String(options.sourceHeightMm),sheetWidthMm:String(options.sheetWidthMm),sheetHeightMm:String(options.sheetHeightMm),copies:String(options.copies??1),dpi:String(options.dpi??300),gapMm:String(options.gapMm??5),marginMm:String(options.marginMm??0),allowRotate:String(options.allowRotate??true)});if(options.fillSheet)query.set("fillSheet","true");return requestBlob(`/v1/production/gang/render?${query}`,artwork)},
  productionCapabilities:()=>request<{icc:boolean;vectorTrace:boolean;polygonBoolean:boolean;maxRenderPixels:number;vipsPath:string;potracePath:string;screeningModes:string[];trapPresets:{id:string;label:string;method:string;spreadPixels:number;underbaseChokePixels:number;threshold:number;notes:string;dpi:number}[];namedInks:{id:string;name:string;hex:string;family:string}[];iccProfiles:boolean;qualityPolicy:string;productionReady:boolean;requireIcc:boolean;requireApproval:boolean;acceptanceGates:{method:string;checks:{id:string;label:string;required:boolean}[]}[];commonIccProfiles:{id:string;label:string;description:string;roles:string[]}[];iccCombinations:{id:string;label:string;sourceProfile:string;destinationProfile:string}[]}>("/v1/production/capabilities"),
  productionGates:()=>request<{gates:{method:string;checks:{id:string;label:string;required:boolean}[]}[]}>("/v1/production/gates"),
  productionRenderScene:async(input:{name:string;method:string;dpi?:number;cropToContent?:boolean;view:{canvasWidth:number;canvasHeight:number;physicalWidthMm:number;physicalHeightMm:number;bleedMm?:number};elements:Record<string,unknown>[]})=>{
    const response=await fetch(`${base}/v1/production/render/scene`,{method:"POST",credentials:"include",headers:{"Content-Type":"application/json",...authHeaders()},body:JSON.stringify(input)});
    if(response.status===401){handleUnauthorized();const body=await response.json().catch(()=>({}));throw new Error(body.message??"Session expired — sign in again")}
    if(!response.ok){const body=await response.json().catch(()=>({}));throw new Error(body.message??`Scene render failed (${response.status})`)}
    const blob=await response.blob();
    return{blob,sha256:response.headers.get("X-PrintStudio-SHA256")??"",widthPx:Number(response.headers.get("X-PrintStudio-Width-Px")??0),heightPx:Number(response.headers.get("X-PrintStudio-Height-Px")??0)};
  },
  createProductionProof:(input:{designId:string;designVersion:number;method:string;artifactSha256:string;widthMm:number;heightMm:number;checklist:Record<string,boolean>;notes?:string})=>request<{id:string;status:string}>("/v1/production/proofs",{method:"POST",body:JSON.stringify(input)}),
  approveProductionProof:(id:string)=>request<{id:string;status:string;frozen:boolean}>(`/v1/production/proofs/${id}/approve`,{method:"POST",body:"{}"}),
  productionBoolean:(subject:PolygonPaths,clip:PolygonPaths,operation:"union"|"difference"|"intersection"|"xor")=>request<{paths:PolygonPaths}>("/v1/production/vector/boolean",{method:"POST",body:JSON.stringify({subject,clip,operation})}),
  productionOffset:(paths:PolygonPaths,deltaMm:number,join:"round"|"square"|"miter"="round",miterLimit=2)=>request<{paths:PolygonPaths}>("/v1/production/vector/offset",{method:"POST",body:JSON.stringify({paths,deltaMm,join,miterLimit})}),
  productionVectorize:async(artwork:Blob,options:{method?:"vinyl"|"embroidery"|"screen"|string;mlPrep?:boolean;alphaCutoff?:number;placement?:VectorizePlacement;assetId?:string}):Promise<VectorContourSet>=>{
    if(options.assetId){
      return request<VectorContourSet>("/v1/production/vectorize",{method:"POST",body:JSON.stringify({assetId:options.assetId,method:options.method??"vinyl",mlPrep:!!options.mlPrep,alphaCutoff:options.alphaCutoff,placement:options.placement})});
    }
    const query=new URLSearchParams({method:options.method??"vinyl",mlPrep:options.mlPrep?"true":"false"});
    if(options.alphaCutoff!=null)query.set("alphaCutoff",String(options.alphaCutoff));
    const headers:Record<string,string>={"Content-Type":artwork.type||"image/png"};
    if(options.placement)headers["X-PrintStudio-Placement"]=JSON.stringify(options.placement);
    const response=await fetch(`${base}/v1/production/vectorize?${query}`,{method:"POST",credentials:"include",headers:{...authHeaders(headers)},body:artwork});
    if(response.status===401){handleUnauthorized();const body=await response.json().catch(()=>({}));throw new Error(body.message??"Session expired — sign in again")}
    const body=await response.json().catch(()=>({}));
    if(!response.ok)throw new Error(body.message??body.error??`Vectorize failed (${response.status})`);
    return body as VectorContourSet;
  },
  productionSpotMatch:(colors:string[],maxDeltaE=6)=>request<{matches:{ink:{id:string;name:string;hex:string;family:string};deltaE00:number;exact:boolean}[];library:string}>("/v1/production/spot/match",{method:"POST",body:JSON.stringify({colors,maxDeltaE})}),
  productionAngleCheck:(angles:{cyan:number;magenta:number;yellow:number;black:number})=>request<{ok:boolean;conflicts:{channelA:string;channelB:string;deltaDegrees:number;severity:string;message:string}[]}>("/v1/production/screen/angles",{method:"POST",body:JSON.stringify(angles)}),
  productionDTFPack:(artwork:Blob,options:{name:string;widthMm:number;heightMm:number;dpi?:number;spread?:number;threshold?:number;trapPreset?:string;proofId?:string})=>{const query=new URLSearchParams({name:options.name,widthMm:String(options.widthMm),heightMm:String(options.heightMm),dpi:String(options.dpi??300),spread:String(options.spread??2),threshold:String(options.threshold??1)});if(options.trapPreset)query.set("trapPreset",options.trapPreset);if(options.proofId)query.set("proofId",options.proofId);return requestBlob(`/v1/production/dtf/pack?${query}`,artwork)},
  productionSublimationPack:(artwork:Blob,options:{name:string;widthMm:number;heightMm:number;dpi?:number;trapPreset?:string;proofId?:string;sourceProfile?:string;destinationProfile?:string})=>{const query=new URLSearchParams({name:options.name,widthMm:String(options.widthMm),heightMm:String(options.heightMm),dpi:String(options.dpi??300),trapPreset:options.trapPreset??"sublimation-paper-standard"});if(options.proofId)query.set("proofId",options.proofId);if(options.sourceProfile)query.set("sourceProfile",options.sourceProfile);if(options.destinationProfile)query.set("destinationProfile",options.destinationProfile);return requestBlob(`/v1/production/sublimation/pack?${query}`,artwork)},
  listMemberships:()=>request<{userId:string;email:string;displayName:string;role:string;kind:string;createdAt?:string}[]>("/v1/memberships"),
  inviteMembership:(email:string,role:string)=>request<{userId:string;email:string;role:string;kind:string}>("/v1/memberships",{method:"POST",body:JSON.stringify({email,role})}),
  updateMembership:(userId:string,role:string)=>request<{userId:string;role:string}>(`/v1/memberships/${userId}`,{method:"PATCH",body:JSON.stringify({role})}),
  removeMembership:(userId:string)=>request<{removed:string;kind:string}>(`/v1/memberships/${userId}`,{method:"DELETE"}),
  productionHalftone:(artwork:Blob,dpi=300,lpi=45,angle=22.5,gamma=1,mode:"am"|"fm"="am")=>requestBlob(`/v1/production/screen/halftone?dpi=${dpi}&lpi=${lpi}&angle=${angle}&gamma=${gamma}&mode=${mode}`,artwork),
  productionScreenPack:(artwork:Blob,options:{name:string;widthMm:number;heightMm:number;dpi?:number;lpi?:number;gamma?:number;underbaseChoke?:number;screening?:"am"|"fm";trapPreset?:string;requireIcc?:boolean;allowUncalibrated?:boolean;sourceProfile?:string;destinationProfile?:string;proofId?:string})=>{const query=new URLSearchParams({name:options.name,widthMm:String(options.widthMm),heightMm:String(options.heightMm),dpi:String(options.dpi??300),lpi:String(options.lpi??45),gamma:String(options.gamma??1),underbaseChoke:String(options.underbaseChoke??-2),screening:options.screening??"am"});if(options.trapPreset)query.set("trapPreset",options.trapPreset);if(options.requireIcc)query.set("requireIcc","true");if(options.allowUncalibrated)query.set("allowUncalibrated","true");if(options.sourceProfile)query.set("sourceProfile",options.sourceProfile);if(options.destinationProfile)query.set("destinationProfile",options.destinationProfile);if(options.proofId)query.set("proofId",options.proofId);return requestBlob(`/v1/production/screen/pack?${query}`,artwork)},
};
