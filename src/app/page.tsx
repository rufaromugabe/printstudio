"use client";
/* eslint-disable @next/next/no-img-element -- previews use generated blob URLs and signed uploads. */

import { ChangeEvent, PointerEvent, useEffect, useRef, useState } from "react";
import { api, EmbroideryCompilation, EmbroideryRequest, Product } from "@/lib/api";
import { GoogleLogin } from "@/components/google-login";
import { digitizeElements } from "@/lib/embroidery-digitizer";
import { prepareProductionExport, ProductionMethod, ProductionResult } from "@/lib/production-export";
import { artifactBlob, createGangSheet, createPDF, createProductionPackage, createTIFF, ExportRecord, listArtifacts, rasterizeArtifact, recordArtifact } from "@/lib/production-packaging";

type Side = string;
type Element = { id: string; type: "text" | "image"; value: string; assetId?: string; sourceWidth?:number; sourceHeight?:number; x: number; y: number; w: number; h: number; rotation: number; color: string; fontSize: number;fontFamily?:string;fontWeight?:number;fontStyle?:"normal"|"italic";textDecoration?:"none"|"underline";textAlign?:"left"|"center"|"right";letterSpacing?:number;lineHeight?:number;strokeColor?:string;strokeWidth?:number;shadow?:boolean;curveType?:"straight"|"circle";curveRadius?:number;curveSweep?:number;curveDirection?:"clockwise"|"counterclockwise";curvePosition?:"outside"|"inside";embroideryKind?:"auto"|"running"|"tatami"|"satin";embroiderySpacing?:number;embroideryAngle?:number;embroideryUnderlay?:"auto"|"none"|"edge"|"center-zigzag" };
type Design = { name: string; product: string; productId?:string;productProperties:Record<string,string|number|boolean>; color: string; method: string; side: Side; elements: Record<Side, Element[]> };

const COLORS = ["#f4f1e9", "#17191c", "#d8b7ab", "#c8cfbc", "#203d63"];
const initial: Design = { name: "Untitled design", product: "Classic Tee",productId:"classic-tee",productProperties:{size:"M",fit:"regular",fabric:"cotton"}, color: "#f4f1e9", method: "DTF", side: "front", elements: { front: [{ id: "welcome", type: "text", value: "MAKE IT YOURS", x: 115, y: 155, w: 190, h: 55, rotation: 0, color: "#222222", fontSize: 24 }], back: [] } };

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
  const [uploadState, setUploadState] = useState("");
  const [zoom, setZoom] = useState(.9);
  const [embroidery,setEmbroidery]=useState<EmbroideryCompilation|null>(null);
  const [embroideryState,setEmbroideryState]=useState<"idle"|"compiling"|"exporting"|"error">("idle");
  const [embroideryError,setEmbroideryError]=useState("");
  const [digitizationFallbacks,setDigitizationFallbacks]=useState(0);
  const [production,setProduction]=useState<ProductionResult|null>(null);
  const [productionState,setProductionState]=useState<"idle"|"preparing"|"error">("idle");
  const [productionError,setProductionError]=useState("");
  const [mirrorVinyl,setMirrorVinyl]=useState(true);
  const [exportHistory,setExportHistory]=useState<ExportRecord[]>([]);
  const [formatState,setFormatState]=useState("");
  const [gangCopies,setGangCopies]=useState(2);
  const [gangWidth,setGangWidth]=useState(300);
  const [gangHeight,setGangHeight]=useState(400);
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

  useEffect(() => {
    if (!hydrated.current || saved) return;
    setCloudState("saving");
    const timer = window.setTimeout(async () => {
      localStorage.setItem("printstudio-design", JSON.stringify(design));
      try { const result = cloudId ? await api.update(cloudId,cloudVersion,design.name,design) : await api.create(design.name,design); setCloudId(result.id);setCloudVersion(result.version);setSaved(true);setCloudState("saved"); }
      catch { setCloudState("error"); }
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
  const upload = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]; if (!file) return;
    setUploadState("Validating upload…");
    try { const asset=await api.uploadAsset(file);const ratio=asset.width/asset.height;const w=ratio>=1?180:180*ratio;const h=ratio>=1?180/ratio:180;const el: Element = { id: crypto.randomUUID(), type: "image", value: asset.url, assetId:asset.id,sourceWidth:asset.width,sourceHeight:asset.height, x: 120, y: 110, w, h, rotation: 0, color: "", fontSize: 0 };commit((d) => ({ ...d, elements: { ...d.elements, [d.side]: [...d.elements[d.side], el] } }));setSelected(el.id);setUploadState(`${asset.width} × ${asset.height}px validated`); }
    catch(error){setUploadState(error instanceof Error?error.message:"Upload rejected");}finally{event.target.value=""}
  };
  const save = async () => { localStorage.setItem("printstudio-design", JSON.stringify(design));setCloudState("saving");try{const result=cloudId?await api.update(cloudId,cloudVersion,design.name,design):await api.create(design.name,design);setCloudId(result.id);setCloudVersion(result.version);setCloudState("saved");setSaved(true)}catch{setCloudState("error")} };
  const share = async () => { if(!cloudId){await save();return}try{const result=await api.share(cloudId);await navigator.clipboard.writeText(`${location.origin}/share/${result.token}`);alert("Share link copied. It expires in 7 days.")}catch{alert("Save the design online before sharing.")} };
  const undo = () => { const last = history.at(-1); if (!last) return; setFuture([design, ...future]); setDesign(last); setHistory(history.slice(0, -1)); };
  const redo = () => { const next = future[0]; if (!next) return; setHistory([...history, design]); setDesign(next); setFuture(future.slice(1)); };
  const remove = () => { if (!selected) return; commit((d) => ({ ...d, elements: { ...d.elements, [d.side]: d.elements[d.side].filter((e) => e.id !== selected) } })); setSelected(null); };
  const safeX=currentView.safeMarginMm/currentView.physicalWidthMm*currentView.canvasWidth;const safeY=currentView.safeMarginMm/currentView.physicalHeightMm*currentView.canvasHeight;
  const warnings = active.filter((e) => e.x < safeX || e.y < safeY || e.x + e.w > currentView.canvasWidth-safeX || e.y + e.h > currentView.canvasHeight-safeY).length;
  const embroideryRequest=async():Promise<EmbroideryRequest>=>{
    const result=await digitizeElements(active,currentView);setDigitizationFallbacks(result.fallbacks.length);
    return{name:design.name,regions:result.regions,machine:{id:"generic-130x180",name:"Generic 130 x 180 mm",hoopWidthMm:130,hoopHeightMm:180,maxStitches:100000,maxColors:16,minStitchMm:.4,maxStitchMm:12.1,maxJumpMm:12.1}};
  };
  const openEmbroidery=async()=>{if(design.method.toLowerCase()!=="embroidery"){setEmbroideryError("Select Embroidery as the decoration method first.");setEmbroidery(null);return}if(!active.length){setEmbroideryError("Add at least one design element before compiling.");setEmbroidery(null);return}setEmbroideryState("compiling");setEmbroideryError("");try{setEmbroidery(await api.compileEmbroidery(await embroideryRequest()));setEmbroideryState("idle")}catch(error){setEmbroideryError(error instanceof Error?error.message:"Compilation failed");setEmbroideryState("error")}};
  const downloadEmbroidery=async()=>{setEmbroideryState("exporting");try{const blob=await api.exportEmbroidery(await embroideryRequest());const url=URL.createObjectURL(blob),link=document.createElement("a");link.href=url;link.download=`${design.name.replace(/[^a-z0-9_-]+/gi,"-")||"printstudio-design"}.dst`;link.click();URL.revokeObjectURL(url);setEmbroideryState("idle")}catch(error){setEmbroideryError(error instanceof Error?error.message:"Export failed");setEmbroideryState("error")}};
  const productionMethod=():ProductionMethod|null=>{const method=design.method.toLowerCase();if(method==="dtf")return"DTF";if(method.includes("vinyl"))return"Vinyl";if(method.includes("screen"))return"Screen print";if(method.includes("sublimation"))return"Sublimation";return null};
  const prepareProduction=async(mirror=mirrorVinyl)=>{const method=productionMethod();if(!method){setProductionError(`${design.method} production export is not implemented yet.`);return}setProductionState("preparing");setProductionError("");if(production)URL.revokeObjectURL(production.previewUrl);try{setProduction(await prepareProductionExport(method,design.name,active,currentView,mirror));setProductionState("idle")}catch(error){setProduction(null);setProductionError(error instanceof Error?error.message:"Production export failed");setProductionState("error")}};
  const exportDesign=()=>{if(design.method.toLowerCase()==="embroidery")void openEmbroidery();else void prepareProduction()};
  const closeProduction=()=>{if(production)URL.revokeObjectURL(production.previewUrl);setProduction(null);setProductionError("");setProductionState("idle")};
  const downloadBlob=async(blob:Blob,fileName:string)=>{if(!production)return;const url=URL.createObjectURL(blob),link=document.createElement("a");link.href=url;link.download=fileName;link.click();window.setTimeout(()=>URL.revokeObjectURL(url),1000);await recordArtifact(production,blob,fileName);setExportHistory(await listArtifacts())};
  const downloadProduction=()=>{if(production)void downloadBlob(production.blob,production.fileName)};
  const createAlternate=async(format:"pdf"|"tiff"|"zip"|"gang")=>{if(!production)return;setFormatState(format);setProductionError("");try{const stem=production.fileName.replace(/\.[^.]+$/,"");if(format==="pdf")await downloadBlob(await createPDF(production),`${stem}.pdf`);if(format==="tiff")await downloadBlob(await createTIFF(production),`${stem}.tiff`);if(format==="zip")await downloadBlob(await createProductionPackage(production),`${stem}-package.zip`);if(format==="gang")await downloadBlob(await createGangSheet(production,gangWidth,gangHeight,gangCopies),`${stem}-gang-${gangWidth}x${gangHeight}mm.png`)}catch(error){setProductionError(error instanceof Error?error.message:"Format generation failed")}finally{setFormatState("")}};
  const createAdvanced=async(kind:"underbase"|"halftone"|"cmyk")=>{if(!production)return;setFormatState(kind);setProductionError("");try{const source=await rasterizeArtifact(production),stem=production.fileName.replace(/\.[^.]+$/,"");if(kind==="underbase")await downloadBlob(await api.productionUnderbase(source,2),`${stem}-white-underbase-spread2.png`);if(kind==="halftone")await downloadBlob(await api.productionHalftone(source,300,45,22.5,1),`${stem}-45lpi-22.5deg-halftone.png`);if(kind==="cmyk")await downloadBlob(await api.productionCMYK(source),`${stem}-cmyk-separations.zip`)}catch(error){setProductionError(error instanceof Error?error.message:"Production processing failed")}finally{setFormatState("")}};
  const redownload=async(record:ExportRecord)=>{const artifact=await artifactBlob(record.id);if(!artifact)return;const url=URL.createObjectURL(artifact.blob),link=document.createElement("a");link.href=url;link.download=artifact.fileName;link.click();window.setTimeout(()=>URL.revokeObjectURL(url),1000)};

  if(!authenticated&&googleClientId)return <GoogleLogin clientId={googleClientId} onSuccess={token=>{localStorage.setItem("printstudio-google-token",token);setAuthenticated(true);location.reload()}}/>;

  return <main className="app-shell">
    <header className="topbar">
      <div className="brand"><span className="brand-mark">P</span><strong>PrintStudio</strong><span className="beta">BETA</span></div>
      <input className="design-name" value={design.name} onChange={(e) => setDesign({ ...design, name: e.target.value })} aria-label="Design name" />
      <div className="top-actions"><span className={cloudState==="saved" ? "status saved" : "status"}>{cloudState==="saving"?"Saving…":cloudState==="saved"?"✓ Cloud saved":cloudState==="error"?"Offline copy":"Local only"}</span><button className="icon-button" onClick={undo} disabled={!history.length}>↶</button><button className="icon-button" onClick={redo} disabled={!future.length}>↷</button><button className="button secondary" onClick={share}>Share</button><button className="button secondary" onClick={save}>Save</button><button className="button primary" onClick={exportDesign}>Export <span>↗</span></button></div>
    </header>
    <section className="workspace">
      <aside className="rail">
        <button className="rail-item active"><span>✦</span>Design</button><button className="rail-item"><span>▦</span>Templates</button><button className="rail-item"><span>♢</span>Elements</button><button className="rail-item" onClick={() => fileRef.current?.click()}><span>⇧</span>Uploads</button><button className="rail-item"><span>AI</span>Imagine</button>
        <div className="rail-bottom"><button className="rail-item"><span>?</span>Help</button></div>
      </aside>
      <aside className="panel">
        <p className="eyebrow">PRODUCT</p><div className="product-card"><div className="mini-shirt">T</div><div><select className="product-select" value={design.productId??design.product} onChange={e=>{const p=products.find(x=>x.id===e.target.value);if(!p)return;const elements={...design.elements};p.template.views.forEach(v=>{elements[v.id]??=[]});const props=Object.fromEntries(p.template.properties.map(x=>[x.id,x.options?.[0]?.value??""]));setDesign({...design,product:p.name,productId:p.id,productProperties:props,side:p.template.views[0]?.id??"front",elements,method:p.methods[0]??design.method,color:p.template.colors[0]?.value??design.color});setSaved(false)}}><option value="classic-tee">Classic Tee</option>{products.filter(p=>p.id!=="classic-tee").map(p=><option value={p.id} key={p.id}>{p.name}</option>)}</select><small>{selectedProduct?.template.category??"Custom product"} · {configuredViews.length} views</small></div></div>
        <div className="field-row"><label>Decoration method<select value={design.method} onChange={(e) => {setDesign({ ...design, method: e.target.value });setSaved(false)}}>{(selectedProduct?.methods??["DTF","Embroidery","Screen print","Vinyl"]).map(method=><option key={method}>{method}</option>)}</select></label>{selectedProduct?.template.properties.map(property=><label key={property.id}>{property.label}{property.type==="select"?<select value={String((design.productProperties??{})[property.id]??property.options[0]?.value??"")} onChange={e=>{setDesign({...design,productProperties:{...(design.productProperties??{}),[property.id]:e.target.value}});setSaved(false)}}>{property.options.map(option=><option value={option.value} key={option.value}>{option.label}</option>)}</select>:<input value={String((design.productProperties??{})[property.id]??"")} type={property.type==="number"?"number":"text"} onChange={e=>{setDesign({...design,productProperties:{...(design.productProperties??{}),[property.id]:e.target.value}});setSaved(false)}}/>}</label>)}</div>
        <p className="eyebrow spaced">ADD TO YOUR DESIGN</p><button className="tool-card" onClick={addText}><span className="tool-icon">T</span><span><strong>Add text</strong><small>Headings, names & slogans</small></span><b>＋</b></button>
        <button className="tool-card" onClick={() => fileRef.current?.click()}><span className="tool-icon">⇧</span><span><strong>Upload artwork</strong><small>{uploadState||"Verified PNG or JPG · max 25 MB"}</small></span><b>＋</b></button><input ref={fileRef} type="file" hidden accept="image/png,image/jpeg" onChange={upload}/>
        <button className="ai-card" disabled title="Planned for a future release"><span>✦</span><div><strong>Create with AI</strong><small>Coming in a future release</small></div><em>SOON</em></button>
        <p className="eyebrow spaced">PRODUCT COLOUR</p><div className="swatches">{(selectedProduct?.template.colors?.length?selectedProduct.template.colors.map(c=>c.value):COLORS).map((color) => <button key={color} aria-label={color} className={design.color === color ? "swatch selected" : "swatch"} style={{ background: color }} onClick={() => {setDesign({ ...design, color });setSaved(false)}}/>)}</div>
        <div className="tip"><span>⌁</span><p><strong>Print tip</strong><br/>Keep important details inside the dotted safe area.</p></div>
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
    {(embroidery||embroideryError||embroideryState==="compiling")&&<div className="embroidery-backdrop" onMouseDown={e=>{if(e.target===e.currentTarget){setEmbroidery(null);setEmbroideryError("")}}}><section className="embroidery-dialog" role="dialog" aria-modal="true" aria-label="Embroidery production preview"><header><div><p className="eyebrow">EMBROIDERY COMPILER</p><h2>Production stitch preview</h2></div><button onClick={()=>{setEmbroidery(null);setEmbroideryError("")}} aria-label="Close">×</button></header>{embroideryState==="compiling"?<div className="embroidery-loading">Tracing artwork and compiling stitch plan…</div>:embroideryError&&!embroidery?<div className="embroidery-failure"><strong>Cannot compile this design</strong><p>{embroideryError}</p></div>:embroidery&&<><div className="embroidery-layout"><div className="stitch-preview" dangerouslySetInnerHTML={{__html:embroidery.svg}}/><div className="embroidery-report"><strong>{embroidery.document.plan.reduce((n,b)=>n+b.underlay.length+b.stitches.length,0).toLocaleString()} commands</strong><small>Compiler {embroidery.document.compilerVersion}<br/>Source {embroidery.document.sourceHash.slice(0,12)}</small><p className={digitizationFallbacks?"approximation-note":"trace-note"}>{digitizationFallbacks?`${digitizationFallbacks} layer${digitizationFallbacks>1?"s":""} could not be decoded and used a boundary fallback.`:"Transparent artwork and rendered text were traced into production contours, including enclosed holes."}</p>{embroidery.document.diagnostics.length?<ul>{embroidery.document.diagnostics.map((d,i)=><li className={d.severity} key={`${d.code}-${i}`}><b>{d.code}</b>{d.message}</li>)}</ul>:<div className="embroidery-ok">✓ Machine-profile checks passed</div>}</div></div>{embroideryError&&<p className="inline-error">{embroideryError}</p>}<footer><button className="button secondary" onClick={()=>{setEmbroidery(null);setEmbroideryError("")}}>Close</button><button className="button primary" disabled={embroideryState==="exporting"||embroidery.document.diagnostics.some(d=>d.severity==="error")} onClick={downloadEmbroidery}>{embroideryState==="exporting"?"Preparing DST…":"Download DST"}</button></footer></>}</section></div>}
    {(production||productionError||productionState==="preparing")&&<ProductionDialog production={production} state={productionState} error={productionError} method={design.method} mirrorVinyl={mirrorVinyl} setMirrorVinyl={setMirrorVinyl} prepareProduction={prepareProduction} close={closeProduction} download={downloadProduction} createAlternate={createAlternate} createAdvanced={createAdvanced} formatState={formatState} gang={{copies:gangCopies,width:gangWidth,height:gangHeight,setCopies:setGangCopies,setWidth:setGangWidth,setHeight:setGangHeight}} history={exportHistory} redownload={redownload}/>} 
  </main>;
}

type ProductionDialogProps={production:ProductionResult|null;state:"idle"|"preparing"|"error";error:string;method:string;mirrorVinyl:boolean;setMirrorVinyl:(value:boolean)=>void;prepareProduction:(mirror?:boolean)=>Promise<void>;close:()=>void;download:()=>void;createAlternate:(format:"pdf"|"tiff"|"zip"|"gang")=>Promise<void>;createAdvanced:(kind:"underbase"|"halftone"|"cmyk")=>Promise<void>;formatState:string;gang:{copies:number;width:number;height:number;setCopies:(value:number)=>void;setWidth:(value:number)=>void;setHeight:(value:number)=>void};history:ExportRecord[];redownload:(record:ExportRecord)=>Promise<void>};
function ProductionDialog({production,state,error,method,mirrorVinyl,setMirrorVinyl,prepareProduction,close,download,createAlternate,createAdvanced,formatState,gang,history,redownload}:ProductionDialogProps){return <div className="embroidery-backdrop" onMouseDown={e=>{if(e.target===e.currentTarget)close()}}><section className="production-dialog" role="dialog" aria-modal="true" aria-label="Production export"><header><div><p className="eyebrow">PRODUCTION EXPORT</p><h2>{production?.method??method}</h2></div><button onClick={close} aria-label="Close">×</button></header>{state==="preparing"?<div className="embroidery-loading">Rendering production artwork…</div>:error&&!production?<div className="embroidery-failure"><strong>Cannot prepare this export</strong><p>{error}</p></div>:production&&<><div className="production-layout"><div className="production-preview"><img src={production.previewUrl} alt={`${production.method} production preview`}/></div><div className="production-report"><strong>{production.summary}</strong><small>{production.widthMM.toFixed(1)} × {production.heightMM.toFixed(1)} mm<br/>{production.fileName}</small>{production.method==="Vinyl"&&<label className="mirror-option"><input type="checkbox" checked={mirrorVinyl} onChange={e=>{setMirrorVinyl(e.target.checked);void prepareProduction(e.target.checked)}}/> Mirror for heat transfer</label>}{production.method==="DTF"&&<><button className="processor-button" disabled={Boolean(formatState)} onClick={()=>void createAdvanced("underbase")}>Generate 2 px white underbase</button><div className="gang-controls"><b>Gang sheet</b><div><label>Copies<input type="number" min="1" max="100" value={gang.copies} onChange={e=>gang.setCopies(+e.target.value)}/></label><label>Width mm<input type="number" min="50" value={gang.width} onChange={e=>gang.setWidth(+e.target.value)}/></label><label>Height mm<input type="number" min="50" value={gang.height} onChange={e=>gang.setHeight(+e.target.value)}/></label></div><button className="button secondary" disabled={Boolean(formatState)} onClick={()=>void createAlternate("gang")}>Build gang sheet</button></div></>}{production.method==="Screen print"&&<div className="screen-processors"><button disabled={Boolean(formatState)} onClick={()=>void createAdvanced("halftone")}>45 LPI halftone</button><button disabled={Boolean(formatState)} onClick={()=>void createAdvanced("cmyk")}>CMYK package</button></div>}<div className="format-actions"><button disabled={Boolean(formatState)} onClick={()=>void createAlternate("pdf")}>PDF</button><button disabled={Boolean(formatState)} onClick={()=>void createAlternate("tiff")}>TIFF</button><button disabled={Boolean(formatState)} onClick={()=>void createAlternate("zip")}>Package ZIP</button></div>{error&&<p className="inline-format-error">{error}</p>}{production.warnings.length?<ul>{production.warnings.map((warning,i)=><li key={i}>{warning}</li>)}</ul>:<div className="embroidery-ok">✓ Production checks passed</div>}</div></div>{history.length>0&&<div className="export-history"><div><strong>Recent immutable exports</strong><small>Stored locally with SHA-256</small></div>{history.slice(0,5).map(record=><button key={record.id} onClick={()=>void redownload(record)}><span>{record.fileName}</span><small>{new Date(record.createdAt).toLocaleString()} · {record.sha256.slice(0,12)}</small></button>)}</div>}<footer><button className="button secondary" onClick={close}>Close</button><button className="button primary" onClick={download}>Download {production.mime==="image/png"?"PNG":"SVG"}</button></footer></>}</section></div>}

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
