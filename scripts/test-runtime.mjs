import { execFileSync, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

// Run with node --env-file=.env scripts/test-runtime.mjs. Secrets stay in the
// inherited process environment; never interpolate them into command arguments.
const env={...process.env,SG1_TEST_DOCKER:"1",SG1_TEST_DATABASE_URL:process.env.SG1_TEST_DATABASE_URL||"postgres://sg1:sg1_local_only@127.0.0.1:55432/sg1?sslmode=disable"};
if(!env.DOCKER_HOST)env.DOCKER_HOST=execFileSync("docker",["context","inspect","--format","{{.Endpoints.docker.Host}}"],{encoding:"utf8"}).trim();
const result=spawnSync("go",["test","-race","-count=1","-v","./internal/platform"],{cwd:fileURLToPath(new URL("../sg-1/backend/",import.meta.url)),env,stdio:"inherit"});
if(result.error){console.error("Could not run Go runtime tests");process.exit(1)}
process.exit(result.status??1);
