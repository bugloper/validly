// Builds the app with a marker API key and fails if the key appears in any
// asset served to browsers.
import { execSync } from "node:child_process";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

const marker = "nextjs-example-secret-marker-93f1";
execSync("next build", { stdio: "inherit", env: { ...process.env, VALIDLY_API_KEY: marker } });

const walk = (dir) =>
  readdirSync(dir).flatMap((f) => (statSync(join(dir, f)).isDirectory() ? walk(join(dir, f)) : [join(dir, f)]));
const leaks = walk(".next/static").filter((f) => readFileSync(f, "utf8").includes(marker));
if (leaks.length) {
  console.error("API key found in browser assets:", leaks);
  process.exit(1);
}
console.log("OK: the API key is not present in browser assets.");
