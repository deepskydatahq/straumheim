---
name: test-crawler
description: Use when testing the website crawler on a URL - runs Firecrawl map, analyzes classification, optionally crawls sample pages
---

# Test Crawler Skill

Programmatically test the website crawling pipeline on any URL. Useful for validating classification logic, checking content quality, and identifying crawl issues before deploying changes.

## When to Use

- Testing changes to `convex/lib/urlUtils.ts`
- Validating crawler behavior on a new target site
- Debugging why pages are being skipped or misclassified
- Checking content quality from Firecrawl

## Prerequisites

- `FIRECRAWL_API_KEY` must be set in `server/.env`
- Firecrawl SDK installed (`@mendable/firecrawl-js`)

## Workflow

### 1. Map the Site

Call Firecrawl's map API to discover all URLs:

```javascript
const response = await fetch("https://api.firecrawl.dev/v1/map", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "Authorization": `Bearer ${process.env.FIRECRAWL_API_KEY}`,
  },
  body: JSON.stringify({ url: targetUrl }),
});
const { links } = await response.json();
```

**Cost:** 1 Firecrawl credit

### 2. Analyze Classification

Run the filtering logic from `urlUtils.ts` on discovered URLs:

- Count URLs by classification type
- Identify what gets skipped and why
- Check for false positives (templates, help articles, localized pages)
- Verify high-value pages are correctly identified

**Key metrics to report:**
- Total URLs discovered
- Same domain vs. different domain
- Skipped breakdown (blog, templates, localized, help, etc.)
- Classification by type (homepage, pricing, features, etc.)
- Final 30 selected for crawling

### 3. Crawl Sample (Optional)

If classification looks good, batch scrape the selected pages:

```javascript
// Start batch scrape
const scrapeResponse = await fetch("https://api.firecrawl.dev/v1/batch/scrape", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "Authorization": `Bearer ${apiKey}`,
  },
  body: JSON.stringify({ urls: selectedUrls, formats: ["markdown"] }),
});
const { id } = await scrapeResponse.json();

// Poll for completion
const statusResponse = await fetch(
  `https://api.firecrawl.dev/v1/batch/scrape/${id}`,
  { headers: { "Authorization": `Bearer ${apiKey}` } }
);
```

**Cost:** 1 credit per page crawled

### 4. Analyze Content Quality

For crawled pages, check:

- Word count (>200 words = good content)
- Heading structure
- Presence of links, lists, tables
- Clean markdown output

## Example Output

```
=== MIRO.COM CRAWL ANALYSIS ===

Total URLs: 5000
After filtering: 970
Docs site found: https://help.miro.com/...

--- SKIPPED BREAKDOWN ---
  /templates/: 2898
  /blog/: 628
  localized: 361
  help-subdomain: 72

--- BY PAGE TYPE ---
  other: 819
  homepage: 1
  pricing: 1
  about: 1

=== FINAL 30 PAGES ===
1. [homepage] https://miro.com
2. [pricing] https://miro.com/pricing
3. [enterprise] https://miro.com/enterprise
...
```

## Common Issues

| Issue | Cause | Fix |
|-------|-------|-----|
| False "homepage" hits | Subdomains matching root path | Pass `rootHostname` to `classifyPageType` |
| Templates in selection | Missing `/templates/` skip pattern | Add to `SKIP_PATTERNS` |
| Localized duplicates | Missing locale prefix skip | Add locales to `LOCALIZED_PREFIXES` |
| Help articles in pricing | Using "contains" instead of strict match | Use regex `^/(pricing)(\/|$)` |

## Iteration Loop

1. Run analysis on target URL
2. Identify classification problems
3. Update `urlUtils.ts`
4. Run `npm test -- --run convex/lib/urlUtils` to verify
5. Re-run analysis to confirm fix
6. Optionally crawl sample to verify content quality
