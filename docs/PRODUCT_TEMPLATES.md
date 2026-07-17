# Flexible product templates

Products are data, not editor code. An administrator submits a product with its name, supported decoration methods and a versioned `template`. The editor builds its view tabs, physical validation, colours and property controls from this document.

```json
{
  "id": "premium-hoodie",
  "name": "Premium Hoodie",
  "methods": ["DTF", "Embroidery", "Screen print"],
  "views": ["front", "back", "left_sleeve", "right_sleeve", "hood"],
  "active": true,
  "template": {
    "version": 1,
    "category": "apparel",
    "views": [
      {
        "id": "front",
        "label": "Front",
        "canvasWidth": 420,
        "canvasHeight": 460,
        "physicalWidthMm": 300,
        "physicalHeightMm": 400,
        "safeMarginMm": 8,
        "bleedMm": 3,
        "mockup": {
          "kind": "shirt",
          "baseAssetId": null,
          "maskAssetId": null,
          "shadowAssetId": null,
          "highlightAssetId": null
        }
      }
    ],
    "properties": [
      {
        "id": "size",
        "label": "Size",
        "type": "select",
        "required": true,
        "options": [
          { "value": "m", "label": "Medium" },
          { "value": "l", "label": "Large" }
        ]
      }
    ],
    "colors": [
      { "value": "#17191c", "label": "Black" }
    ]
  }
}
```

## Supported flexibility

- One to twenty named views: garment sides, sleeves, hood, cap panels, mug wraps, box faces or any future surface.
- Independent logical canvas and physical millimetre dimensions per view.
- Independent safe margin and bleed per view.
- Mockup base, mask, shadow and highlight asset references per view.
- Arbitrary `select`, `text`, `number` and `boolean` product properties.
- Product-specific colours and decoration methods.
- Versioned templates, allowing future schema upgrades without silently changing existing orders.

IDs must start with a lowercase letter and contain only lowercase letters, numbers and underscores. Canvas dimensions are limited to 50–4000 logical units and physical dimensions to 3000 mm. The API rejects duplicate views/properties, invalid dimensions, unsupported property types and empty select lists.

Existing designs retain their selected property values and element collections keyed by view ID. Orders should snapshot both the product template version and design version so later product edits cannot alter a placed order.
