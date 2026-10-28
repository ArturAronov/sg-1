import { randomBytes } from "node:crypto";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const root = new URL("../", import.meta.url);
const destination = new URL(".env", root);
if (existsSync(destination)) {
  console.log(".env already exists; it was not changed.");
} else {
  const template = readFileSync(new URL(".env.example", root), "utf8");
  writeFileSync(destination, template.replace("SG1_DEV_TOKEN=\n", `SG1_DEV_TOKEN=${randomBytes(32).toString("hex")}\n`), { flag: "wx", mode: 0o600 });
  console.log(`Created ${fileURLToPath(destination)} with a random development token.`);
  console.log("Keep this file private. Use its SG1_DEV_TOKEN to sign in locally.");
}
