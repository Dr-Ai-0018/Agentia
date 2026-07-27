import fs from "node:fs";
import path from "node:path";

const dist = process.env.DIST_DIR
  ? path.resolve(process.env.DIST_DIR)
  : path.resolve(import.meta.dirname, "..", "dist");
const assets = path.join(dist, "assets");
const bundles = fs.readdirSync(assets).filter((name) => /^index-.*\.js$/.test(name));

if (bundles.length !== 1) {
  throw new Error(`expected exactly one production JS bundle, found ${bundles.length}`);
}

const source = fs.readFileSync(path.join(assets, bundles[0]), "utf8");
const mockMarkers = ["演示中", "Mode = mock", "不是真实 backend"];
const found = mockMarkers.filter((marker) => source.includes(marker));

if (found.length > 0) {
  throw new Error(`refusing mock production artifact; found: ${found.join(", ")}`);
}

process.stdout.write(`verified HTTP production artifact: ${bundles[0]}\n`);
