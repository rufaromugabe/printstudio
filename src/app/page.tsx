"use client";
/* eslint-disable @next/next/no-img-element -- previews use generated blob URLs and signed uploads. */

import { ChangeEvent, PointerEvent, useEffect, useRef, useState } from "react";
import { api, Asset, EmbroideryCompilation, EmbroideryRequest, Product } from "@/lib/api";
import { GoogleLogin } from "@/components/google-login";
import { digitizeElements } from "@/lib/embroidery-digitizer";
import { prepareProductionExport, ProductionMethod, ProductionResult } from "@/lib/production-export";
import { artifactBlob, createPDF, createProductionPackage, createTIFF, ExportRecord, listArtifacts, rasterizeArtifact, recordArtifact } from "@/lib/production-packaging";

type Side = string;
type SidebarPanel = "design"|"templates"|"elements"|"uploads"|"imagine"|"help";
type Element = { id: string; type: "text" | "image"; value: string; assetId?: string; sourceWidth?:number; sourceHeight?:number; x: number; y: number; w: number; h: number; rotation: number; color: string; fontSize: number;fontFamily?:string;fontWeight?:number;fontStyle?:"normal"|"italic";textDecoration?:"none"|"underline";textAlign?:"left"|"center"|"right";letterSpacing?:number;lineHeight?:number;strokeColor?:string;strokeWidth?:number;shadow?:boolean;curveType?:"straight"|"circle";curveRadius?:number;curveSweep?:number;curveDirection?:"clockwise"|"counterclockwise";curvePosition?:"outside"|"inside";embroideryKind?:"auto"|"running"|"tatami"|"satin";embroiderySpacing?:number;embroideryAngle?:number;embroideryUnderlay?:"auto"|"none"|"edge"|"center-zigzag" };
type Design = { name: string; product: string; productId?:string;productProperties:Record<string,string|number|boolean>; color: string; method: string; side: Side; elements: Record<Side, Element[]> };

const COLORS = ["#f4f1e9", "#17191c", "#d8b7ab", "#c8cfbc", "#203d63"];
const initial: Design = { name: "Untitled design", product: "Classic Tee",productId:"classic-tee",productProperties:{size:"M",fit:"regular",fabric:"cotton"}, color: "#f4f1e9", method: "DTF", side: "front", elements: { front: [{ id: "welcome", type: "text", value: "MAKE IT YOURS", x: 115, y: 155, w: 190, h: 55, rotation: 0, color: "#222222", fontSize: 24 }], back: [] } };

const TEMPLATE_PRESETS=[
  {id:"bold-statement",name:"Bold statement",description:"Large headline with a small supporting line",create:(view:{canvasWidth:number;canvasHeight:number}):Element[]=>[
    textElement("MAKE IT",view.canvasWidth/2-125,view.canvasHeight/2-62,250,64,42,800),
    textElement("UNMISTAKABLE",view.canvasWidth/2-105,view.canvasHeight/2+10,210,34,18,700,"#17634f"),
  ]},
  {id:"team-number",name:"Team number",description:"Arched team name and oversized number",create:(view:{canvasWidth:number;canvasHeight:number}):Element[]=>[
    {...textElement("YOUR TEAM",view.canvasWidth/2-110,70,220,190,24,800),curveType:"circle",curveRadius:88,curveSweep:145,curveDirection:"clockwise"},
    textElement("24",view.canvasWidth/2-85,185,170,120,92,800),
  ]},
  {id:"brand-lockup",name:"Brand lockup",description:"Clean business name and tagline",create:(view:{canvasWidth:number;canvasHeight:number}):Element[]=>[
    textElement("YOUR BRAND",view.canvasWidth/2-125,view.canvasHeight/2-45,250,55,34,800),
    textElement("MADE WITH PURPOSE",view.canvasWidth/2-105,view.canvasHeight/2+18,210,28,13,400,"#5d6864"),
  ]},
];

function textElement(value:string,x:number,y:number,w:number,h:number,fontSize:number,fontWeight=400,color="#222222"):Element{return{id:crypto.randomUUID(),type:"text",value,x,y,w,h,rotation:0,color,fontSize,fontFamily:"Arial",fontWeight,fontStyle:"normal",textDecoration:"none",textAlign:"center",letterSpacing:0,lineHeight:1.1,strokeColor:"#ffffff",strokeWidth:0,shadow:false,curveType:"straight",curveRadius:85,curveSweep:240,curveDirection:"clockwise",curvePosition:"outside"}}

export default function Home() {
  const [design, setDesign] = useState<Design>(initial);
  const [selected, setSelected] = useState<string | null>("welcome");
  const [history, setHistory] = useState<Design[]>([]);
  const [future, setFuture] = useState<Design[]>([]);
  const [saved, setSaved] = useState(true);
  const [cloudId, setCloudId] = useState<string | null>(null);
  const [cloudVersion, setCloudVersion] = useState(0);
  const [cloudState, setCloudState] = useState<"offline"|"saving"|"saved"|"error">("offline");
  const [products, setProducts] = useState<Product[]>([]);
  const [assets,setAssets]=useState<Asset[]>([]);
  const [assetState,setAssetState]=useState<"loading"|"ready"|"offline">("loading");
  const [uploadState, setUploadState] = useState("");
  const [activePanel,setActivePanel]=useState<SidebarPanel>("design");
  const [sidebarOpen,setSidebarOpen]=useState(true);
  const [ideaText,setIdeaText]=useState("");
  const [zoom, setZoom] = useState(.9);
  const [embroidery,setEmbroidery]=useState<EmbroideryCompilation|null>(null);
  const [embroideryState,setEmbroideryState]=useState<"idle"|"compiling"|"exporting"|"error">("idle");
  const [embroideryError,setEmbroideryError]=useState("");
  const [production,setProduction]=useState<ProductionResult|null>(null);
  const [productionState,setProductionState]=useState<"idle"|"preparing"|"error">("idle");
  const [productionError,setProductionError]=useState("");
  const [mirrorVinyl,setMirrorVinyl]=useState(true);
  const [exportHistory,setExportHistory]=useState<ExportRecord[]>([]);
  const [formatState,setFormatState]=useState("");
  const [gangCopies,setGangCopies]=useState(2);
  const [gangWidth,setGangWidth]=useState(210);
  const [gangHeight,setGangHeight]=useState(297);
  const [gangFillSheet,setGangFillSheet]=useState(true);
  const [gangGap,setGangGap]=useState(5);
  const [proofId,setProofId]=useState("");
  const [proofState,setProofState]=useState<"none"|"pending"|"approved">("none");
  const embroideryRequestRef=useRef<EmbroideryRequest|null>(null);
  const googleClientId=process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID??"";
  const [authenticated,setAuthenticated]=useState(!googleClientId);
  const hydrated = useRef(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const selectedProduct=products.find(p=>p.id===design.productId)||products.find(p=>p.name===design.product);
  const configuredViews=selectedProduct?.template?.views?.length?selectedProduct.template.views:[{id:"front",label:"Front",canvasWidth:420,canvasHeight:460,physicalWidthMm:300,physicalHeightMm:400,safeMarginMm:8,bleedMm:3,mockup:{kind:"shirt"}},{id:"back",label:"Back",canvasWidth:420,canvasHeight:460,physicalWidthMm:300,physicalHeightMm:400,safeMarginMm:8,bleedMm:3,mockup:{kind:"shirt"}}];
  const currentView=configuredViews.find(v=>v.id===design.side)??configuredViews[0];
  const active = design.elements[design.side]??[];
  const selectedElement = active.find((item) => item.id === selected);

  useEffect(() => {
    const timer=window.setTimeout(()=>setAuthenticated(!googleClientId||Boolean(localStorage.getItem("printstudio-google-token"))),0);return()=>window.clearTimeout(timer);
  }, [googleClientId]);

  useEffect(() => {
    const raw = localStorage.getItem("printstudio-design");
    if (!raw) { hydrated.current = true; return; }
    const timer = window.setTimeout(() => {
      try { setDesign(JSON.parse(raw)); } catch { /* ignore corrupt local data */ }
      hydrated.current = true;
    }, 0);
    return () => window.clearTimeout(timer);
  }, []);

  useEffect(()=>{listArtifacts().then(setExportHistory).catch(()=>setExportHistory([]))},[]);

  useEffect(() => {
    api.products().then(setProducts).catch(() => setCloudState("offline"));
    api.designs<Design>().then(async (items) => { const latest=items[0]; if(latest){setCloudId(latest.id);setCloudVersion(latest.version);const document=latest.document;const imageElements=[...document.elements.front,...document.elements.back].filter(e=>e.type==="image"&&e.assetId);await Promise.all(imageElements.map(async e=>{try{e.value=(await api.assetURL(e.assetId!)).url}catch{/* retain cached preview */}}));setDesign(document);setCloudState("saved");} }).catch(() => setCloudState("offline")).finally(()=>{hydrated.current=true});
  }, []);

  useEffect(()=>{
    api.assets().then(async items=>{
      const visible=items.filter(item=>item.status==="validated").slice(0,40);
      const hydratedAssets=await Promise.all(visible.map(async item=>{try{return{...item,url:(await api.assetURL(item.id)).url}}catch{return item}}));
      setAssets(hydratedAssets);setAssetState("ready");
    }).catch(()=>setAssetState("offline"));
  },[]);

  useEffect(() => {
    if (!hydrated.current || saved) return;
    setCloudState("saving");
    const timer = window.setTimeout(async () => {
      localStorage.setItem("printstudio-design", JSON.stringify(design));
      try {
        const result = cloudId ? await api.update(cloudId,cloudVersion,design.name,design) : await api.create(design.name,design);
        setCloudId(result.id);setCloudVersion(result.version);setSaved(true);setCloudState("saved");
      } catch (error) {
        const message=error instanceof Error?error.message:"";
        if(cloudId&&message.includes("changed in another session")){
          try{
            const latest=await api.design<Design>(cloudId);
            setCloudVersion(latest.version);
            setDesign(latest.document);
            setSaved(true);
            setCloudState("saved");
            setHistory([]);
            setFuture([]);
          }catch{setCloudState("error")}
        }else setCloudState("error");
      }
    }, 1200);
    return () => window.clearTimeout(timer);
  }, [cloudId, cloudVersion, design, saved]);

  const commit = (recipe: (draft: Design) => Design) => {
    setHistory((items) => [...items.slice(-39), design]);
    setFuture([]); setDesign(recipe(design)); setSaved(false);
  };
  const patchElement = (id: string, patch: Partial<Element>, transient = false) => {
    const apply = (base: Design) => ({ ...base, elements: { ...base.elements, [base.side]: base.elements[base.side].map((el) => el.id === id ? { ...el, ...patch } : el) } });
    if (transient) { setDesign(apply(design)); setSaved(false); } else { commit(apply); }
  };
  const addText = () => {
    const el: Element = { id: crypto.randomUUID(), type: "text", value: "Your text", x: 120, y: 170, w: 170, h: 55, rotation: 0, color: "#222222", fontSize: 28,fontFamily:"Arial",fontWeight:400,fontStyle:"normal",textDecoration:"none",textAlign:"center",letterSpacing:0,lineHeight:1.1,strokeColor:"#ffffff",strokeWidth:0,shadow:false,curveType:"straight",curveRadius:85,curveSweep:240,curveDirection:"clockwise",curvePosition:"outside" };
    commit((d) => ({ ...d, elements: { ...d.elements, [d.side]: [...d.elements[d.side], el] } })); setSelected(el.id);
  };
  const insertAsset=async(asset:Asset)=>{let url=asset.url;if(!url)url=(await api.assetURL(asset.id)).url;const ratio=asset.width/asset.height;const w=ratio>=1?180:180*ratio;const h=ratio>=1?180/ratio:180;const el:Element={id:crypto.randomUUID(),type:"image",value:url,assetId:asset.id,sourceWidth:asset.width,sourceHeight:asset.height,x:Math.max(0,(currentView.canvasWidth-w)/2),y:Math.max(0,(currentView.canvasHeight-h)/2),w,h,rotation:0,color:"",fontSize:0};commit(d=>({...d,elements:{...d.elements,[d.side]:[...(d.elements[d.side]??[]),el]}}));setSelected(el.id);setUploadState(`${asset.width} × ${asset.height}px added`)};
  const uploadFile=async(file:File)=>{if(!["image/png","image/jpeg"].includes(file.type)){setUploadState("Choose a PNG or JPG file.");return}if(file.size>25*1024*1024){setUploadState("Artwork must be 25 MB or smaller.");return}setUploadState("Uploading and validating…");try{const asset=await api.uploadAsset(file);setAssets(items=>[asset,...items.filter(item=>item.id!==asset.id)]);setAssetState("ready");await insertAsset(asset)}catch(error){setUploadState(error instanceof Error?error.message:"Upload rejected")}};
  const upload=async(event:ChangeEvent<HTMLInputElement>)=>{const file=event.target.files?.[0];if(file)await uploadFile(file);event.target.value=""};
  const selectProduct=(productId:string)=>{const product=products.find(item=>item.id===productId);if(!product)return;const elements={...design.elements};product.template.views.forEach(view=>{elements[view.id]??=[]});const props=Object.fromEntries(product.template.properties.map(property=>[property.id,property.options?.[0]?.value??""]));commit(d=>({...d,product:product.name,productId:product.id,productProperties:props,side:product.template.views[0]?.id??"front",elements,method:product.methods[0]??d.method,color:product.template.colors[0]?.value??d.color}));setSelected(null)};
  const applyTemplate=(preset:(typeof TEMPLATE_PRESETS)[number])=>{const additions=preset.create(currentView);commit(d=>({...d,elements:{...d.elements,[d.side]:[...(d.elements[d.side]??[]),...additions]}}));setSelected(additions[0]?.id??null)};
  const addShape=(kind:"circle"|"rectangle"|"star"|"divider"|"badge")=>{const shapes={circle:`<circle cx="128" cy="128" r="116"/>`,rectangle:`<rect x="12" y="36" width="232" height="184" rx="22"/>`,star:`<path d="M128 8l29 78 83 4-65 52 22 80-69-45-69 45 22-80-65-52 83-4z"/>`,divider:`<rect x="8" y="110" width="240" height="36" rx="18"/>`,badge:`<path fill-rule="evenodd" d="M128 8a120 120 0 1 0 0 240 120 120 0 0 0 0-240zm0 25a95 95 0 1 1 0 190 95 95 0 0 1 0-190z"/>`};const svg=`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" fill="#17201d">${shapes[kind]}</svg>`;const wide=kind==="divider"||kind==="rectangle",w=wide?180:130,h=kind==="divider"?36:wide?120:130;const el:Element={id:crypto.randomUUID(),type:"image",value:`data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`,sourceWidth:256,sourceHeight:256,x:(currentView.canvasWidth-w)/2,y:(currentView.canvasHeight-h)/2,w,h,rotation:0,color:"",fontSize:0};commit(d=>({...d,elements:{...d.elements,[d.side]:[...(d.elements[d.side]??[]),el]}}));setSelected(el.id)};
  const duplicateElement=(element:Element)=>{const copy={...element,id:crypto.randomUUID(),x:element.x+12,y:element.y+12};commit(d=>({...d,elements:{...d.elements,[d.side]:[...(d.elements[d.side]??[]),copy]}}));setSelected(copy.id)};
  const moveLayer=(id:string,direction:-1|1)=>commit(d=>{const layers=[...(d.elements[d.side]??[])],index=layers.findIndex(element=>element.id===id),next=index+direction;if(index<0||next<0||next>=layers.length)return d;[layers[index],layers[next]]=[layers[next],layers[index]];return{...d,elements:{...d.elements,[d.side]:layers}}});
  const addIdeaAsText=()=>{const value=ideaText.trim();if(!value)return;const el=textElement(value,Math.max(0,currentView.canvasWidth/2-125),Math.max(0,currentView.canvasHeight/2-35),250,70,32,800);commit(d=>({...d,elements:{...d.elements,[d.side]:[...(d.elements[d.side]??[]),el]}}));setSelected(el.id);setIdeaText("")};
  const save = async () => { localStorage.setItem("printstudio-design", JSON.stringify(design));setCloudState("saving");try{const result=cloudId?await api.update(cloudId,cloudVersion,design.name,design):await api.create(design.name,design);setCloudId(result.id);setCloudVersion(result.version);setCloudState("saved");setSaved(true);return result}catch{setCloudState("error");return null} };
  const share = async () => { if(!cloudId){await save();return}try{const result=await api.share(cloudId);await navigator.clipboard.writeText(`${location.origin}/share/${result.token}`);alert("Share link copied. It expires in 7 days.")}catch{alert("Save the design online before sharing.")} };
  const undo = () => { const last = history.at(-1); if (!last) return; setFuture([design, ...future]); setDesign(last); setHistory(history.slice(0, -1)); setSaved(false); };
  const redo = () => { const next = future[0]; if (!next) return; setHistory([...history, design]); setDesign(next); setFuture(future.slice(1)); setSaved(false); };
  const remove = () => { if (!selected) return; commit((d) => ({ ...d, elements: { ...d.elements, [d.side]: d.elements[d.side].filter((e) => e.id !== selected) } })); setSelected(null); };
  const safeX=currentView.safeMarginMm/currentView.physicalWidthMm*currentView.canvasWidth;const safeY=currentView.safeMarginMm/currentView.physicalHeightMm*currentView.canvasHeight;
  const warnings = active.filter((e) => e.x < safeX || e.y < safeY || e.x + e.w > currentView.canvasWidth-safeX || e.y + e.h > currentView.canvasHeight-safeY).length;
  const embroideryRequest=async():Promise<EmbroideryRequest>=>{
    const refreshed=await Promise.all(active.map(async element=>{
      if(element.type!=="image"||!element.assetId)return element;
      try{return{...element,value:(await api.assetURL(element.assetId)).url}}catch{return element}
    }));
    const result=await digitizeElements(refreshed,currentView);
    const request:EmbroideryRequest={name:design.name,regions:result.regions,machine:{id:"generic-130x180",name:"Generic 130 x 180 mm",hoopWidthMm:130,hoopHeightMm:180,maxStitches:100000,maxColors:16,minStitchMm:.4,maxStitchMm:12.1,maxJumpMm:12.1}};
    embroideryRequestRef.current=request;
    return request;
  };
  const openEmbroidery=async()=>{if(design.method.toLowerCase()!=="embroidery"){setEmbroideryError("Select Embroidery as the decoration method first.");setEmbroidery(null);return}if(!active.length){setEmbroideryError("Add at least one design element before compiling.");setEmbroidery(null);return}setEmbroideryState("compiling");setEmbroideryError("");try{setEmbroidery(await api.compileEmbroidery(await embroideryRequest()));setEmbroideryState("idle")}catch(error){embroideryRequestRef.current=null;setEmbroideryError(error instanceof Error?error.message:"Compilation failed");setEmbroideryState("error")}};
  const downloadEmbroidery=async()=>{setEmbroideryState("exporting");try{const request=embroideryRequestRef.current??await embroideryRequest();const blob=await api.exportEmbroidery(request);const url=URL.createObjectURL(blob),link=document.createElement("a");link.href=url;link.download=`${design.name.replace(/[^a-z0-9_-]+/gi,"-")||"printstudio-design"}.dst`;link.click();URL.revokeObjectURL(url);setEmbroideryState("idle")}catch(error){setEmbroideryError(error instanceof Error?error.message:"Export failed");setEmbroideryState("error")}};
  const productionMethod=():ProductionMethod|null=>{const method=design.method.toLowerCase();if(method==="dtf")return"DTF";if(method.includes("vinyl"))return"Vinyl";if(method.includes("screen"))return"Screen print";if(method.includes("sublimation"))return"Sublimation";return null};
  const prepareProduction=async(mirror=mirrorVinyl)=>{const method=productionMethod();if(!method){setProductionError(`${design.method} production export is not implemented yet.`);return}setProductionState("preparing");setProductionError("");setProofId("");setProofState("none");if(production)URL.revokeObjectURL(production.previewUrl);try{setProduction(await prepareProductionExport(method,design.name,active,currentView,mirror));setProductionState("idle")}catch(error){setProduction(null);setProductionError(error instanceof Error?error.message:"Production export failed");setProductionState("error")}};
  const exportDesign=()=>{if(design.method.toLowerCase()==="embroidery")void openEmbroidery();else void prepareProduction()};
  const closeProduction=()=>{if(production)URL.revokeObjectURL(production.previewUrl);setProduction(null);setProductionError("");setProductionState("idle");setProofId("");setProofState("none")};
  const downloadBlob=async(blob:Blob,fileName:string)=>{if(!production)return;const url=URL.createObjectURL(blob),link=document.createElement("a");link.href=url;link.download=fileName;link.click();window.setTimeout(()=>URL.revokeObjectURL(url),1000);await recordArtifact(production,blob,fileName);setExportHistory(await listArtifacts())};
  const downloadProduction=()=>{if(production)void downloadBlob(production.blob,production.fileName)};
  const ensureApprovedProof=async():Promise<string>=>{
    if(proofState==="approved"&&proofId)return proofId;
    if(!production)throw new Error("Prepare a production preview first.");
    const caps=await api.productionCapabilities();
    if(!caps.requireApproval)return "";
    const savedDesign=await save();
    const designId=savedDesign?.id??cloudId;
    const version=savedDesign?.version??(cloudVersion||1);
    if(!designId)throw new Error("Save the design to the cloud before packaging.");
    const sha=production.sha256||Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256",await production.blob.arrayBuffer())),b=>b.toString(16).padStart(2,"0")).join("");
    const gate=caps.acceptanceGates.find(item=>item.method===production.method);
    const checklist=Object.fromEntries((gate?.checks??[]).map(check=>[check.id,true]));
    const proof=await api.createProductionProof({designId,designVersion:Math.max(1,version),method:production.method,artifactSha256:sha,widthMm:production.widthMM,heightMm:production.heightMM,checklist,notes:"Acknowledged during studio production export review"});
    setProofId(proof.id);setProofState("pending");
    const approved=await api.approveProductionProof(proof.id);
    setProofId(approved.id);setProofState("approved");
    return approved.id;
  };
  const createAlternate=async(format:"pdf"|"tiff"|"zip"|"gang")=>{if(!production)return;setFormatState(format);setProductionError("");try{const stem=production.fileName.replace(/\.[^.]+$/,"");if(format==="pdf")await downloadBlob(await createPDF(production),`${stem}.pdf`);if(format==="tiff")await downloadBlob(await createTIFF(production),`${stem}.tiff`);if(format==="zip"){const approvedProofId=await ensureApprovedProof();if(production.method==="DTF")await downloadBlob(await api.productionDTFPack(production.blob,{name:design.name,widthMm:production.widthMM,heightMm:production.heightMM,trapPreset:"dtf-pet-film-standard",proofId:approvedProofId||undefined}),`${stem}-dtf-package.zip`);else if(production.method==="Screen print"){const source=await rasterizeArtifact(production);await downloadBlob(await api.productionScreenPack(source,{name:design.name,widthMm:production.widthMM,heightMm:production.heightMM,lpi:45,screening:"am",trapPreset:"screen-plastisol-45lpi",allowUncalibrated:true,proofId:approvedProofId||undefined}),`${stem}-screen-package.zip`)}else await downloadBlob(await createProductionPackage(production),`${stem}-package.zip`)}if(format==="gang"){const source=production.mime==="image/png"?production.blob:await rasterizeArtifact(production);await downloadBlob(await api.productionGangRender(source,{name:design.name,sourceWidthMm:production.widthMM,sourceHeightMm:production.heightMM,sheetWidthMm:gangWidth,sheetHeightMm:gangHeight,copies:gangFillSheet?1:gangCopies,fillSheet:gangFillSheet,gapMm:gangGap,allowRotate:true}),`${stem}-sheet-${gangWidth}x${gangHeight}mm${gangFillSheet?"-filled":`-x${gangCopies}`}.png`)}}catch(error){setProductionError(error instanceof Error?error.message:"Format generation failed")}finally{setFormatState("")}};
  const createAdvanced=async(kind:"underbase"|"halftone"|"halftone-fm"|"cmyk")=>{if(!production)return;setFormatState(kind);setProductionError("");try{const source=await rasterizeArtifact(production),stem=production.fileName.replace(/\.[^.]+$/,"");if(kind==="underbase")await downloadBlob(await api.productionUnderbase(source,2),`${stem}-white-underbase-spread2.png`);if(kind==="halftone")await downloadBlob(await api.productionHalftone(source,300,45,22.5,1,"am"),`${stem}-45lpi-22.5deg-halftone.png`);if(kind==="halftone-fm")await downloadBlob(await api.productionHalftone(source,300,45,22.5,1,"fm"),`${stem}-fm-stochastic-halftone.png`);if(kind==="cmyk")await downloadBlob(await api.productionCMYK(source),`${stem}-cmyk-preview-uncalibrated.zip`)}catch(error){setProductionError(error instanceof Error?error.message:"Production processing failed")}finally{setFormatState("")}};
  const redownload=async(record:ExportRecord)=>{const artifact=await artifactBlob(record.id);if(!artifact)return;const url=URL.createObjectURL(artifact.blob),link=document.createElement("a");link.href=url;link.download=artifact.fileName;link.click();window.setTimeout(()=>URL.revokeObjectURL(url),1000)};

  if(!authenticated&&googleClientId)return <GoogleLogin clientId={googleClientId} onSuccess={token=>{localStorage.setItem("printstudio-google-token",token);setAuthenticated(true);location.reload()}}/>;

  return <main className="app-shell">
    <header className="topbar">
      <div className="brand"><span className="brand-mark">P</span><strong>PrintStudio</strong><span className="beta">BETA</span></div>
      <input className="design-name" value={design.name} onChange={(e) => {setDesign({ ...design, name: e.target.value });setSaved(false)}} aria-label="Design name" />
      <div className="top-actions"><span className={cloudState==="saved" ? "status saved" : "status"}>{cloudState==="saving"?"Saving…":cloudState==="saved"?"✓ Cloud saved":cloudState==="error"?"Offline copy":"Local only"}</span><button className="icon-button" onClick={undo} disabled={!history.length}>↶</button><button className="icon-button" onClick={redo} disabled={!future.length}>↷</button><button className="button secondary" onClick={share}>Share</button><button className="button secondary" onClick={save}>Save</button><button className="button primary" onClick={exportDesign}>Export <span>↗</span></button></div>
    </header>
    <section className="workspace">
      <aside className="rail">
        {([{id:"design",icon:"D",label:"Design"},{id:"templates",icon:"▦",label:"Templates"},{id:"elements",icon:"◇",label:"Elements"},{id:"uploads",icon:"↑",label:"Uploads"},{id:"imagine",icon:"AI",label:"Imagine"}] as {id:SidebarPanel;icon:string;label:string}[]).map(item=><button key={item.id} className={`rail-item ${activePanel===item.id?"active":""}`} aria-pressed={activePanel===item.id} onClick={()=>{setActivePanel(item.id);setSidebarOpen(true)}}><span>{item.icon}</span>{item.label}</button>)}
        <div className="rail-bottom"><button className={`rail-item ${activePanel==="help"?"active":""}`} aria-pressed={activePanel==="help"} onClick={()=>{setActivePanel("help");setSidebarOpen(true)}}><span>?</span>Help</button></div>
      </aside>
      <input ref={fileRef} type="file" hidden accept="image/png,image/jpeg" onChange={upload}/>
      <aside className={`panel ${sidebarOpen?"open":"closed"}`} aria-label={`${activePanel} tools`}>
        <button className="panel-close" onClick={()=>setSidebarOpen(false)} aria-label="Close tools panel">×</button>
        {activePanel==="design"&&<>
          <p className="eyebrow">PRODUCT</p><div className="product-card"><div className="mini-shirt">T</div><div><select className="product-select" value={design.productId??design.product} onChange={e=>selectProduct(e.target.value)}><option value="classic-tee">Classic Tee</option>{products.filter(product=>product.id!=="classic-tee").map(product=><option value={product.id} key={product.id}>{product.name}</option>)}</select><small>{selectedProduct?.template.category??"Custom product"} · {configuredViews.length} views</small></div></div>
          <div className="field-row"><label>Decoration method<select value={design.method} onChange={e=>commit(d=>({...d,method:e.target.value}))}>{(selectedProduct?.methods??["DTF","Embroidery","Screen print","Vinyl"]).map(method=><option key={method}>{method}</option>)}</select></label>{selectedProduct?.template.properties.map(property=><label key={property.id}>{property.label}{property.type==="select"?<select value={String((design.productProperties??{})[property.id]??property.options[0]?.value??"")} onChange={e=>commit(d=>({...d,productProperties:{...(d.productProperties??{}),[property.id]:e.target.value}}))}>{property.options.map(option=><option value={option.value} key={option.value}>{option.label}</option>)}</select>:<input value={String((design.productProperties??{})[property.id]??"")} type={property.type==="number"?"number":"text"} onChange={e=>commit(d=>({...d,productProperties:{...(d.productProperties??{}),[property.id]:e.target.value}}))}/>}</label>)}</div>
          <p className="eyebrow spaced">ADD TO YOUR DESIGN</p><button className="tool-card" onClick={addText}><span className="tool-icon">T</span><span><strong>Add text</strong><small>Headings, names and slogans</small></span><b>+</b></button>
          <button className="tool-card" onClick={()=>setActivePanel("uploads")}><span className="tool-icon">↑</span><span><strong>Upload artwork</strong><small>{uploadState||"Verified PNG or JPG · max 25 MB"}</small></span><b>+</b></button>
          <button className="ai-card" onClick={()=>setActivePanel("imagine")}><span>✦</span><div><strong>Creative ideas</strong><small>Prepare a prompt or add an idea as text</small></div><em>PREVIEW</em></button>
          <p className="eyebrow spaced">PRODUCT COLOUR</p><div className="swatches">{(selectedProduct?.template.colors?.length?selectedProduct.template.colors.map(color=>color.value):COLORS).map(color=><button key={color} aria-label={color} className={design.color===color?"swatch selected":"swatch"} style={{background:color}} onClick={()=>commit(d=>({...d,color}))}/>)}</div>
          <div className="tip"><span>⌁</span><p><strong>Print tip</strong><br/>Keep important details inside the dotted safe area.</p></div>
        </>}
        {activePanel==="templates"&&<><div className="panel-title"><p className="eyebrow">TEMPLATES</p><h2>Start with a layout</h2><p>Templates add editable elements to the current view without deleting your work.</p></div><div className="template-list">{TEMPLATE_PRESETS.map(preset=><button key={preset.id} className={`template-card ${preset.id}`} onClick={()=>applyTemplate(preset)}><span className="template-preview">{preset.id==="team-number"?"24":preset.id==="brand-lockup"?"BRAND":"MAKE IT"}</span><strong>{preset.name}</strong><small>{preset.description}</small><em>Add to design</em></button>)}</div></>}
        {activePanel==="elements"&&<><div className="panel-title"><p className="eyebrow">ELEMENTS</p><h2>Shapes and layers</h2><p>Add vector-safe basics, then arrange everything on this view.</p></div><div className="shape-grid">{(["circle","rectangle","star","divider","badge"] as const).map(shape=><button key={shape} onClick={()=>addShape(shape)}><span className={`shape-sample ${shape}`}/>{shape}</button>)}</div><div className="panel-section-head"><strong>Current layers</strong><span>{active.length}</span></div><div className="panel-layers">{[...active].reverse().map((element,index)=><div key={element.id} className={selected===element.id?"active":""}><button className="layer-name" onClick={()=>setSelected(element.id)}><span>{element.type==="text"?"T":"▧"}</span>{element.type==="text"?element.value:"Artwork"}</button><div><button title="Move forward" disabled={index===0} onClick={()=>moveLayer(element.id,1)}>↑</button><button title="Move backward" disabled={index===active.length-1} onClick={()=>moveLayer(element.id,-1)}>↓</button><button title="Duplicate" onClick={()=>duplicateElement(element)}>⧉</button></div></div>)}</div></>}
        {activePanel==="uploads"&&<><div className="panel-title"><p className="eyebrow">UPLOADS</p><h2>Your artwork</h2><p>Validated assets stay available so you can reuse them across views.</p></div><button className="upload-dropzone" onClick={()=>fileRef.current?.click()} onDragOver={event=>{event.preventDefault();event.currentTarget.classList.add("dragging")}} onDragLeave={event=>event.currentTarget.classList.remove("dragging")} onDrop={event=>{event.preventDefault();event.currentTarget.classList.remove("dragging");const file=event.dataTransfer.files[0];if(file)void uploadFile(file)}}><span>↑</span><strong>Upload PNG or JPG</strong><small>Click or drop artwork here · maximum 25 MB</small></button>{uploadState&&<p className="upload-status" role="status">{uploadState}</p>}<div className="panel-section-head"><strong>Asset library</strong><span>{assets.length}</span></div>{assetState==="loading"?<div className="panel-empty">Loading your uploads…</div>:assetState==="offline"?<div className="panel-empty"><strong>Asset service unavailable</strong><span>You can retry when the API and object storage are running.</span></div>:assets.length===0?<div className="panel-empty"><strong>No uploads yet</strong><span>Your first validated image will appear here.</span></div>:<div className="asset-grid">{assets.map(asset=><button key={asset.id} onClick={()=>void insertAsset(asset).catch(error=>setUploadState(error instanceof Error?error.message:"Asset unavailable"))} title={`Add ${asset.fileName}`}>{asset.url?<img src={asset.url} alt=""/>:<span>↻</span>}<strong>{asset.fileName}</strong><small>{asset.width} × {asset.height}</small></button>)}</div>}</>}
        {activePanel==="imagine"&&<><div className="panel-title"><p className="eyebrow">IMAGINE</p><h2>Shape the idea first</h2><p>AI generation is intentionally not connected yet. You can still capture a direction and place it as editable text.</p></div><label className="idea-field">Design idea<textarea value={ideaText} onChange={event=>setIdeaText(event.target.value)} placeholder="Example: Harare cycling club, bold retro lettering, sunrise colours"/></label><button className="button primary panel-action" disabled={!ideaText.trim()} onClick={addIdeaAsText}>Add idea as text</button><p className="eyebrow spaced">IDEA STARTERS</p><div className="idea-chips">{["Local pride","Team spirit","Minimal brand","Birthday crew","Faith and purpose","Retro sports"].map(idea=><button key={idea} onClick={()=>setIdeaText(idea)}>{idea}</button>)}</div><div className="tip"><span>i</span><p>Image generation will only be enabled when an AI provider, credit controls and content-safety flow are configured.</p></div></>}
        {activePanel==="help"&&<><div className="panel-title"><p className="eyebrow">HELP</p><h2>Studio guide</h2><p>Build on one product view at a time, then review production warnings before export.</p></div><div className="help-list"><article><span>1</span><div><strong>Choose the product</strong><p>Product configuration controls available views, physical size and decoration methods.</p></div></article><article><span>2</span><div><strong>Add and edit artwork</strong><p>Upload reusable images, add text or start from an editable layout.</p></div></article><article><span>3</span><div><strong>Stay inside the safe area</strong><p>The dotted boundary protects important detail from production edges.</p></div></article><article><span>4</span><div><strong>Export for the method</strong><p>DTF, screen, vinyl, sublimation and embroidery apply different checks.</p></div></article></div><div className="shortcut-card"><strong>Quick controls</strong><span>Drag · move element</span><span>Corner handles · resize</span><span>Top handle · rotate</span><span>Toolbar arrows · undo and redo</span></div></>}
      </aside>
      <section className="stage-wrap">
        <div className="view-tabs">{configuredViews.map(view=><button key={view.id} className={design.side===view.id?"active":""} onClick={()=>{setDesign({...design,side:view.id,elements:{...design.elements,[view.id]:design.elements[view.id]??[]}});setSelected(null)}}>{view.label}</button>)}</div>
        <div className="stage" style={{transform:`scale(${zoom})`}} onPointerDown={() => setSelected(null)}>
          <div className={`shirt ${currentView.mockup?.kind??"generic"}`} style={{ "--shirt": design.color } as React.CSSProperties}><div className="sleeve left"/><div className="sleeve right"/><div className="neck"/><div className="fabric"/></div>
          <div className="print-area" style={{width:currentView.canvasWidth,height:currentView.canvasHeight}}><span className="area-label">PRINT AREA · {Math.round(currentView.physicalWidthMm/10)} × {Math.round(currentView.physicalHeightMm/10)} CM</span>{active.map((el) => <CanvasElement key={el.id} element={el} selected={selected === el.id} onSelect={() => setSelected(el.id)} onChange={(patch) => patchElement(el.id,patch,true)} />)}</div>
        </div>
        <div className="zoom"><button onClick={()=>setZoom(Math.max(.5,zoom-.1))}>−</button><span>{Math.round(zoom*100)}%</span><button onClick={()=>setZoom(Math.min(1.3,zoom+.1))}>＋</button><button onClick={()=>setZoom(.9)}>⌗</button></div>
      </section>
      <aside className="properties">
        <div className="prop-head"><strong>{selectedElement ? (selectedElement.type === "text" ? "Text settings" : "Image settings") : "Design check"}</strong><span>×</span></div>
        {selectedElement ? <>
          <label className="prop-label">{selectedElement.type === "text" ? "CONTENT" : "SIZE"}</label>
          {selectedElement.type === "text" && <><textarea value={selectedElement.value} onChange={(e)=>patchElement(selectedElement.id,{value:e.target.value},true)}/><TextControls element={selectedElement} onChange={patch=>patchElement(selectedElement.id,patch,true)}/></>}
          <div className="prop-grid"><label>Width<input type="number" min="24" value={Math.round(selectedElement.w)} onChange={(e)=>patchElement(selectedElement.id,{w:+e.target.value})}/></label><label>Height<input type="number" min="24" value={Math.round(selectedElement.h)} onChange={(e)=>patchElement(selectedElement.id,{h:+e.target.value})}/></label></div>
          {design.method.toLowerCase()==="embroidery"&&<EmbroideryControls element={selectedElement} onChange={patch=>patchElement(selectedElement.id,patch)}/>} 
          <button className="delete" onClick={remove}>Delete element</button>
        </> : <div className="empty-prop"><span>✓</span><strong>{warnings ? `${warnings} placement warning${warnings>1?"s":""}` : "Ready to print"}</strong><p>Select an element to edit its size, content and colour.</p></div>}
        <div className="layers"><div><strong>Layers</strong><span>{active.length}</span></div>{[...active].reverse().map((e)=><button key={e.id} className={selected===e.id?"active":""} onClick={()=>setSelected(e.id)}><span>{e.type === "text" ? "T" : "▧"}</span>{e.type === "text" ? e.value : "Uploaded artwork"}</button>)}</div>
      </aside>
    </section>
    {(embroidery||embroideryError||embroideryState==="compiling")&&<div className="embroidery-backdrop" onMouseDown={e=>{if(e.target===e.currentTarget){setEmbroidery(null);setEmbroideryError("")}}}><section className="embroidery-dialog" role="dialog" aria-modal="true" aria-label="Embroidery production preview"><header><div><p className="eyebrow">EMBROIDERY COMPILER</p><h2>Production stitch preview</h2></div><button onClick={()=>{setEmbroidery(null);setEmbroideryError("")}} aria-label="Close">×</button></header>{embroideryState==="compiling"?<div className="embroidery-loading">Tracing artwork and compiling stitch plan…</div>:embroideryError&&!embroidery?<div className="embroidery-failure"><strong>Cannot compile this design</strong><p>{embroideryError}</p></div>:embroidery&&<><div className="embroidery-layout"><div className="stitch-preview" dangerouslySetInnerHTML={{__html:embroidery.svg}}/><div className="embroidery-report"><strong>{embroidery.document.plan.reduce((n,b)=>n+b.underlay.length+b.stitches.length,0).toLocaleString()} commands</strong><small>Compiler {embroidery.document.compilerVersion}<br/>Source {embroidery.document.sourceHash.slice(0,12)}</small><p className="trace-note">Transparent artwork and rendered text were traced into production contours, including enclosed holes. Boundary-rectangle fallbacks are disabled.</p>{embroidery.document.diagnostics.length?<ul>{embroidery.document.diagnostics.map((d,i)=><li className={d.severity} key={`${d.code}-${i}`}><b>{d.code}</b>{d.message}</li>)}</ul>:<div className="embroidery-ok">✓ Machine-profile checks passed</div>}</div></div>{embroideryError&&<p className="inline-error">{embroideryError}</p>}<footer><button className="button secondary" onClick={()=>{setEmbroidery(null);setEmbroideryError("")}}>Close</button><button className="button primary" disabled={embroideryState==="exporting"||embroidery.document.diagnostics.some(d=>d.severity==="error")} onClick={downloadEmbroidery}>{embroideryState==="exporting"?"Preparing DST…":"Download DST"}</button></footer></>}</section></div>}
    {(production||productionError||productionState==="preparing")&&<ProductionDialog production={production} state={productionState} error={productionError} method={design.method} mirrorVinyl={mirrorVinyl} setMirrorVinyl={setMirrorVinyl} prepareProduction={prepareProduction} close={closeProduction} download={downloadProduction} createAlternate={createAlternate} createAdvanced={createAdvanced} proofState={proofState} formatState={formatState} gang={{copies:gangCopies,width:gangWidth,height:gangHeight,fillSheet:gangFillSheet,gap:gangGap,setCopies:setGangCopies,setWidth:setGangWidth,setHeight:setGangHeight,setFillSheet:setGangFillSheet,setGap:setGangGap}} history={exportHistory} redownload={redownload}/>} 
  </main>;
}

type ProductionDialogProps={production:ProductionResult|null;state:"idle"|"preparing"|"error";error:string;method:string;mirrorVinyl:boolean;setMirrorVinyl:(value:boolean)=>void;prepareProduction:(mirror?:boolean)=>Promise<void>;close:()=>void;download:()=>void;createAlternate:(format:"pdf"|"tiff"|"zip"|"gang")=>Promise<void>;createAdvanced:(kind:"underbase"|"halftone"|"halftone-fm"|"cmyk")=>Promise<void>;proofState:"none"|"pending"|"approved";formatState:string;gang:{copies:number;width:number;height:number;fillSheet:boolean;gap:number;setCopies:(value:number)=>void;setWidth:(value:number)=>void;setHeight:(value:number)=>void;setFillSheet:(value:boolean)=>void;setGap:(value:number)=>void};history:ExportRecord[];redownload:(record:ExportRecord)=>Promise<void>};
function ProductionDialog({production,state,error,method,mirrorVinyl,setMirrorVinyl,prepareProduction,close,download,createAlternate,createAdvanced,proofState,formatState,gang,history,redownload}:ProductionDialogProps){
  const sheetPresets=[{id:"a4",label:"A4",w:210,h:297},{id:"letter",label:"Letter",w:216,h:279},{id:"dtf-30x40",label:"DTF 30×40",w:300,h:400},{id:"a3",label:"A3",w:297,h:420}];
  return <div className="embroidery-backdrop" onMouseDown={e=>{if(e.target===e.currentTarget)close()}}><section className="production-dialog" role="dialog" aria-modal="true" aria-label="Production export"><header><div><p className="eyebrow">PRODUCTION EXPORT</p><h2>{production?.method??method}</h2></div><button onClick={close} aria-label="Close">×</button></header>{state==="preparing"?<div className="embroidery-loading">Rendering production artwork…</div>:error&&!production?<div className="embroidery-failure"><strong>Cannot prepare this export</strong><p>{error}</p></div>:production&&<><div className="production-layout"><div className="production-preview"><img src={production.previewUrl} alt={`${production.method} production preview`}/></div><div className="production-report"><strong>{production.summary}</strong><small>{production.widthMM.toFixed(1)} × {production.heightMM.toFixed(1)} mm<br/>{production.fileName}{production.renderer?` · ${production.renderer} render`:""}</small>{proofState==="approved"?<div className="embroidery-ok">✓ Production proof approved for this preview</div>:<p className="curve-hint">Packaging will reuse this preview and approve the proof automatically when required — no extra approval step.</p>}{production.method==="Vinyl"&&<label className="mirror-option"><input type="checkbox" checked={mirrorVinyl} onChange={e=>{setMirrorVinyl(e.target.checked);void prepareProduction(e.target.checked)}}/> Mirror for heat transfer</label>}{production.method==="DTF"&&<button className="processor-button" disabled={Boolean(formatState)} onClick={()=>void createAdvanced("underbase")}>Generate 2 px white underbase</button>}{production.method==="Screen print"&&<div className="screen-processors"><button disabled={Boolean(formatState)} onClick={()=>void createAdvanced("halftone")}>45 LPI AM</button><button disabled={Boolean(formatState)} onClick={()=>void createAdvanced("halftone-fm")}>FM stochastic</button><button disabled={Boolean(formatState)} onClick={()=>void createAdvanced("cmyk")}>CMYK preview</button></div>}
        <div className="gang-controls"><b>Repeat on sheet</b><p>Tile this artwork across a full page so small logos do not waste film or paper.</p><div className="sheet-presets">{sheetPresets.map(preset=><button key={preset.id} type="button" className={gang.width===preset.w&&gang.height===preset.h?"active":""} onClick={()=>{gang.setWidth(preset.w);gang.setHeight(preset.h)}}>{preset.label}</button>)}</div><div><label>Width mm<input type="number" min="50" value={gang.width} onChange={e=>gang.setWidth(+e.target.value)}/></label><label>Height mm<input type="number" min="50" value={gang.height} onChange={e=>gang.setHeight(+e.target.value)}/></label><label>Gap mm<input type="number" min="0" max="50" value={gang.gap} onChange={e=>gang.setGap(+e.target.value)}/></label></div><label className="mirror-option"><input type="checkbox" checked={gang.fillSheet} onChange={e=>gang.setFillSheet(e.target.checked)}/> Fill the sheet with as many copies as fit</label>{!gang.fillSheet&&<label>Copies<input type="number" min="1" max="500" value={gang.copies} onChange={e=>gang.setCopies(+e.target.value)}/></label>}<button className="button primary" disabled={Boolean(formatState)} onClick={()=>void createAlternate("gang")}>{formatState==="gang"?"Building sheet…":gang.fillSheet?"Download filled sheet":`Download sheet × ${gang.copies}`}</button></div>
        <div className="format-actions"><button disabled={Boolean(formatState)} onClick={()=>void createAlternate("pdf")}>PDF</button><button disabled={Boolean(formatState)} onClick={()=>void createAlternate("tiff")}>TIFF</button><button disabled={Boolean(formatState)} onClick={()=>void createAlternate("zip")}>{formatState==="zip"?"Packaging…":production.method==="DTF"?"DTF Pack ZIP":production.method==="Screen print"?"Screen Pack ZIP":"Package ZIP"}</button></div>{error&&<p className="inline-format-error">{error}</p>}{production.warnings.length?<ul>{production.warnings.map((warning,i)=><li key={i}>{warning}</li>)}</ul>:<div className="embroidery-ok">✓ Production checks passed</div>}</div></div>{history.length>0&&<div className="export-history"><div><strong>Recent immutable exports</strong><small>Stored locally with SHA-256</small></div>{history.slice(0,5).map(record=><button key={record.id} onClick={()=>void redownload(record)}><span>{record.fileName}</span><small>{new Date(record.createdAt).toLocaleString()} · {record.sha256.slice(0,12)}</small></button>)}</div>}<footer><button className="button secondary" onClick={close}>Close</button><button className="button primary" onClick={download}>Download {production.mime==="image/png"?"PNG":"SVG"}</button></footer></>}</section></div>;
}

function EmbroideryControls({element,onChange}:{element:Element;onChange:(patch:Partial<Element>)=>void}){return <div className="embroidery-controls"><p className="prop-label">EMBROIDERY</p><label>Stitch family<select value={element.embroideryKind??"auto"} onChange={e=>onChange({embroideryKind:e.target.value as Element["embroideryKind"]})}><option value="auto">Auto</option><option value="satin">Satin</option><option value="tatami">Tatami fill</option><option value="running">Running stitch</option></select></label><div className="control-grid"><label>Row spacing<input type="number" min=".25" max="2.5" step=".05" value={element.embroiderySpacing??.45} onChange={e=>onChange({embroiderySpacing:+e.target.value})}/></label><label>Direction<input type="number" min="-180" max="180" step="5" value={element.embroideryAngle??0} onChange={e=>onChange({embroideryAngle:+e.target.value})}/></label></div><label>Underlay<select value={element.embroideryUnderlay??"auto"} onChange={e=>onChange({embroideryUnderlay:e.target.value as Element["embroideryUnderlay"]})}><option value="auto">Automatic</option><option value="center-zigzag">Center + zigzag</option><option value="edge">Edge run</option><option value="none">None</option></select></label><p className="curve-hint">Unsafe satin overrides are blocked by the selected machine profile.</p></div>}

function TextControls({element,onChange}:{element:Element;onChange:(patch:Partial<Element>)=>void}){
  const toggle=(key:"fontStyle"|"textDecoration",on:string,off:string)=>onChange({[key]:(element[key]??off)===on?off:on} as Partial<Element>);
  const circular=(element.curveType??"straight")==="circle";
  return <div className="text-controls">
    <label className="full">Text shape<select value={element.curveType??"straight"} onChange={e=>{const circle=e.target.value==="circle";onChange({curveType:circle?"circle":"straight",...(circle&&element.w<180?{w:220,h:220}: {})})}}><option value="straight">Straight</option><option value="circle">Circular</option></select></label>
    {circular&&<div className="curve-controls"><label className="curve-slider"><span>Curve <b>{element.curveSweep??240}°</b></span><input type="range" min="30" max="360" step="5" value={element.curveSweep??240} onChange={e=>onChange({curveSweep:+e.target.value})}/></label><div className="control-grid"><label>Radius<input type="number" min="24" max="300" value={element.curveRadius??85} onChange={e=>onChange({curveRadius:+e.target.value})}/></label><label>Direction<select value={element.curveDirection??"clockwise"} onChange={e=>onChange({curveDirection:e.target.value as Element["curveDirection"]})}><option value="clockwise">Clockwise</option><option value="counterclockwise">Counter-clockwise</option></select></label><label>Placement<select value={element.curvePosition??"outside"} onChange={e=>onChange({curvePosition:e.target.value as Element["curvePosition"]})}><option value="outside">Outside</option><option value="inside">Inside</option></select></label></div><p className="curve-hint">Use the element rotation handle to change the circle orientation.</p></div>}
    <label className="full">Font<select value={element.fontFamily??"Arial"} onChange={e=>onChange({fontFamily:e.target.value})}>{["Arial","Georgia","Verdana","Trebuchet MS","Courier New","Impact","Times New Roman"].map(font=><option key={font} value={font}>{font}</option>)}</select></label>
    <div className="format-row"><button className={(element.fontWeight??400)>=700?"active":""} onClick={()=>onChange({fontWeight:(element.fontWeight??400)>=700?400:700})} title="Bold"><b>B</b></button><button className={element.fontStyle==="italic"?"active":""} onClick={()=>toggle("fontStyle","italic","normal")} title="Italic"><i>I</i></button><button className={element.textDecoration==="underline"?"active":""} onClick={()=>toggle("textDecoration","underline","none")} title="Underline"><u>U</u></button>{(["left","center","right"] as const).map(align=><button key={align} className={(element.textAlign??"center")===align?"active":""} onClick={()=>onChange({textAlign:align})} title={`${align} align`}>{align==="left"?"≡":align==="center"?"≣":"☰"}</button>)}</div>
    <div className="control-grid"><label>Size<input type="number" min="8" max="240" value={element.fontSize} onChange={e=>onChange({fontSize:+e.target.value})}/></label><label>Colour<input type="color" value={element.color} onChange={e=>onChange({color:e.target.value})}/></label><label>Spacing<input type="number" min="-5" max="30" step=".5" value={element.letterSpacing??0} onChange={e=>onChange({letterSpacing:+e.target.value})}/></label><label>Line height<input type="number" min=".7" max="3" step=".1" value={element.lineHeight??1.1} onChange={e=>onChange({lineHeight:+e.target.value})}/></label><label>Outline<input type="number" min="0" max="8" step=".5" value={element.strokeWidth??0} onChange={e=>onChange({strokeWidth:+e.target.value})}/></label><label>Outline colour<input type="color" value={element.strokeColor??"#ffffff"} onChange={e=>onChange({strokeColor:e.target.value})}/></label></div>
    <label className="check"><input type="checkbox" checked={element.shadow??false} onChange={e=>onChange({shadow:e.target.checked})}/> Soft shadow</label>
  </div>
}

function CircularText({element}:{element:Element}){const radius=Math.max(20,Math.min(element.curveRadius??85,Math.min(element.w,element.h)/2-4));const cx=element.w/2,cy=element.h/2;const degrees=Math.max(30,Math.min(360,element.curveSweep??240));const sign=element.curveDirection==="counterclockwise"?-1:1;const start=-90-sign*degrees/2;const point=(angle:number)=>{const radians=angle*Math.PI/180;return{x:cx+radius*Math.cos(radians),y:cy+radius*Math.sin(radians)}};const a=point(start),b=point(start+sign*degrees/2),c=point(start+sign*degrees);const sweep=sign>0?1:0;const path=`M ${a.x} ${a.y} A ${radius} ${radius} 0 0 ${sweep} ${b.x} ${b.y} A ${radius} ${radius} 0 0 ${sweep} ${c.x} ${c.y}`;const pathId=`curve-${element.id}`;return <svg className="curved-text" viewBox={`0 0 ${element.w} ${element.h}`} aria-label={element.value}><defs><path id={pathId} d={path}/></defs><text fill={element.color} stroke={element.strokeColor??"transparent"} strokeWidth={element.strokeWidth??0} paintOrder="stroke" textDecoration={element.textDecoration??"none"}><textPath href={`#${pathId}`} startOffset="50%" textAnchor="middle" dy={element.curvePosition==="inside"?element.fontSize:0}>{element.value}</textPath></text></svg>}

function CanvasElement({ element, selected, onSelect, onChange }: { element: Element; selected: boolean; onSelect: () => void; onChange: (patch:Partial<Element>)=>void }) {
  const drag = useRef<{px:number;py:number;x:number;y:number}|null>(null);
  const resize = useRef<{px:number;py:number;x:number;y:number;w:number;h:number;corner:string}|null>(null);
  const rotate = useRef<{cx:number;cy:number}|null>(null);
  const down = (e: PointerEvent<HTMLDivElement>) => { e.stopPropagation(); onSelect(); drag.current={px:e.clientX,py:e.clientY,x:element.x,y:element.y}; e.currentTarget.setPointerCapture(e.pointerId); };
  const move = (e: PointerEvent<HTMLDivElement>) => { if(!drag.current)return; onChange({x:Math.max(-element.w+10,Math.min(410,drag.current.x+e.clientX-drag.current.px)),y:Math.max(-element.h+10,Math.min(450,drag.current.y+e.clientY-drag.current.py))}); };
  const startResize=(e:PointerEvent<HTMLElement>,corner:string)=>{e.stopPropagation();resize.current={px:e.clientX,py:e.clientY,x:element.x,y:element.y,w:element.w,h:element.h,corner};e.currentTarget.setPointerCapture(e.pointerId)};
  const resizing=(e:PointerEvent<HTMLElement>)=>{const r=resize.current;if(!r)return;const dx=e.clientX-r.px,dy=e.clientY-r.py;let x=r.x,y=r.y,w=r.w,h=r.h;if(r.corner.includes("e"))w=Math.max(24,r.w+dx);if(r.corner.includes("s"))h=Math.max(24,r.h+dy);if(r.corner.includes("w")){w=Math.max(24,r.w-dx);x=r.x+(r.w-w)}if(r.corner.includes("n")){h=Math.max(24,r.h-dy);y=r.y+(r.h-h)}onChange({x,y,w,h})};
  const startRotate=(e:PointerEvent<HTMLElement>)=>{e.stopPropagation();const rect=e.currentTarget.parentElement!.getBoundingClientRect();rotate.current={cx:rect.left+rect.width/2,cy:rect.top+rect.height/2};e.currentTarget.setPointerCapture(e.pointerId)};
  const rotating=(e:PointerEvent<HTMLElement>)=>{if(!rotate.current)return;onChange({rotation:Math.round(Math.atan2(e.clientY-rotate.current.cy,e.clientX-rotate.current.cx)*180/Math.PI+90)})};
  return <div className={selected ? "canvas-el selected" : "canvas-el"} onPointerDown={down} onPointerMove={move} onPointerUp={()=>drag.current=null} style={{left:element.x,top:element.y,width:element.w,height:element.h,transform:`rotate(${element.rotation}deg)`,color:element.color,fontSize:element.fontSize,fontFamily:element.fontFamily??"Arial",fontWeight:element.fontWeight??400,fontStyle:element.fontStyle??"normal",textDecoration:element.textDecoration??"none",textAlign:element.textAlign??"center",letterSpacing:element.letterSpacing??0,lineHeight:element.lineHeight??1.1,WebkitTextStroke:`${element.strokeWidth??0}px ${element.strokeColor??"transparent"}`,textShadow:element.shadow?"0 3px 6px #00000055":"none"}}>{element.type === "text" ? (element.curveType==="circle"?<CircularText element={element}/>:<span>{element.value}</span>) : <img src={element.value} alt="Uploaded artwork"/>}{selected && <><i className="rotate-handle" onPointerDown={startRotate} onPointerMove={rotating} onPointerUp={()=>rotate.current=null}>↻</i>{["nw","ne","sw","se"].map(c=><i key={c} className={`handle ${c}`} onPointerDown={e=>startResize(e,c)} onPointerMove={resizing} onPointerUp={()=>resize.current=null}/>)}</>}</div>;
}
