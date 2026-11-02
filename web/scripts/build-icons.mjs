// Writes the SVG icon files from the one geometry module, so they cannot drift from the Mark and the Stone.
// Run: node scripts/build-icons.mjs  (PNG fallbacks and the touch icon are rendered from these SVGs)
import { writeFileSync } from "node:fs";
import { faviconSvg, markSvg } from "../src/ui/stoneGeometry.ts";

const here = (p) => new URL(p, import.meta.url);
writeFileSync(here("../public/mark.svg"), markSvg() + "\n");
writeFileSync(here("../public/favicon.svg"), faviconSvg() + "\n");
console.log("wrote public/mark.svg and public/favicon.svg");
