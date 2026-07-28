import assert from "node:assert/strict";
import fs from "node:fs";
import ts from "typescript";

const policySource = fs.readFileSync(new URL("../src/app/pollingPolicy.ts", import.meta.url), "utf8");
const transpiled = ts.transpileModule(policySource.replace(/^import type .*$/m, "type ConsolePage = string;"), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText;
const policy = await import(`data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`);

for (const page of ["overview", "residents", "runs", "system"]) {
  assert.equal(policy.shouldPollSummary(page), true, `${page} should poll summary`);
}
for (const page of ["world-chat", "compaction-diagnostics", "settings"]) {
  assert.equal(policy.shouldPollSummary(page), false, `${page} must not poll full summary`);
}

const appSource = fs.readFileSync(new URL("../src/app/App.tsx", import.meta.url), "utf8");
assert.match(appSource, /new AbortController\(\)/, "App polling must create abortable requests");
assert.match(appSource, /summaryRequest\.current\?\.abort\(\)/, "summary requests must be cancelled");
assert.match(appSource, /chatRequest\.current\?\.abort\(\)/, "chat requests must be cancelled");
assert.match(appSource, /shouldPollSummary\(page\)/, "summary polling must use the page policy");

console.log("app polling tests passed");
