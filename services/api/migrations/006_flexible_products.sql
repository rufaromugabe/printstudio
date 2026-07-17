ALTER TABLE products ADD COLUMN IF NOT EXISTS template jsonb NOT NULL DEFAULT '{"version":1,"views":[],"properties":[],"colors":[]}'::jsonb;

UPDATE products SET template='{
  "version":1,
  "category":"apparel",
  "views":[
    {"id":"front","label":"Front","canvasWidth":420,"canvasHeight":460,"physicalWidthMm":300,"physicalHeightMm":400,"safeMarginMm":8,"bleedMm":3,"mockup":{"kind":"shirt","baseAssetId":null,"maskAssetId":null,"shadowAssetId":null,"highlightAssetId":null}},
    {"id":"back","label":"Back","canvasWidth":420,"canvasHeight":460,"physicalWidthMm":300,"physicalHeightMm":400,"safeMarginMm":8,"bleedMm":3,"mockup":{"kind":"shirt","baseAssetId":null,"maskAssetId":null,"shadowAssetId":null,"highlightAssetId":null}},
    {"id":"left_sleeve","label":"Left sleeve","canvasWidth":240,"canvasHeight":300,"physicalWidthMm":100,"physicalHeightMm":120,"safeMarginMm":5,"bleedMm":2,"mockup":{"kind":"sleeve","baseAssetId":null,"maskAssetId":null,"shadowAssetId":null,"highlightAssetId":null}},
    {"id":"right_sleeve","label":"Right sleeve","canvasWidth":240,"canvasHeight":300,"physicalWidthMm":100,"physicalHeightMm":120,"safeMarginMm":5,"bleedMm":2,"mockup":{"kind":"sleeve","baseAssetId":null,"maskAssetId":null,"shadowAssetId":null,"highlightAssetId":null}}
  ],
  "properties":[
    {"id":"size","label":"Size","type":"select","required":true,"options":[{"value":"S","label":"Small"},{"value":"M","label":"Medium"},{"value":"L","label":"Large"},{"value":"XL","label":"Extra large"}]},
    {"id":"fit","label":"Fit","type":"select","required":true,"options":[{"value":"regular","label":"Regular"},{"value":"oversized","label":"Oversized"}]},
    {"id":"fabric","label":"Fabric","type":"select","required":true,"options":[{"value":"cotton","label":"100% cotton"},{"value":"blend","label":"Cotton blend"}]}
  ],
  "colors":[{"value":"#f4f1e9","label":"Natural"},{"value":"#17191c","label":"Black"},{"value":"#d8b7ab","label":"Rose"},{"value":"#c8cfbc","label":"Sage"},{"value":"#203d63","label":"Navy"}]
}'::jsonb WHERE id='classic-tee';
