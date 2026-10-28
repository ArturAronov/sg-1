import { randomBytes } from "node:crypto";
import { appendFileSync, existsSync, readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const root = new URL("../", import.meta.url);
const destination = new URL(".env", root);
if (existsSync(destination)) {
  const content = readFileSync(destination, "utf8");
  if (!/^SG1_CREDENTIALS_KEY=/m.test(content)) {
    appendFileSync(destination, `\nSG1_CREDENTIALS_KEY=${randomBytes(32).toString("base64")}\n`);
    console.log("Added a separate credential-encryption key to the existing .env; existing values were preserved.");
  } else {
    console.log(".env already exists; it was not changed. Do not replace an existing encryption key.");
  }
} else {
  const template = readFileSync(new URL(".env.example", root), "utf8");
  const content = template.replace("SG1_DEV_TOKEN=\n", `SG1_DEV_TOKEN=${randomBytes(32).toString("hex")}\n`).replace("SG1_CREDENTIALS_KEY=\n", `SG1_CREDENTIALS_KEY=${randomBytes(32).toString("base64")}\n`);
  writeFileSync(destination, content, { flag: "wx", mode: 0o600 });
  console.log(`Created ${fileURLToPath(destination)} with a random development token.`);
  console.log("Keep this file private. Use its SG1_DEV_TOKEN to sign in locally.");
}
