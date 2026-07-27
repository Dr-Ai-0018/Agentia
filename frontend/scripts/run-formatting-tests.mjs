import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import ts from "typescript";

const root = path.resolve(import.meta.dirname, "..");

function loadTsModule(relativePath) {
  const filename = path.join(root, relativePath);
  const source = fs.readFileSync(filename, "utf8");
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
    },
    fileName: filename,
  }).outputText;
  const module = { exports: {} };
  const fn = new Function("exports", "module", compiled);
  fn(module.exports, module);
  return module.exports;
}

const { compactDuration, compactElapsedLabel } = loadTsModule("src/lib/duration.ts");
const { describeDoing, doingPrefix } = loadTsModule("src/features/residents/residentSpeak.ts");
const { interventionFollowups, interventionStatusLabel, priorityLabel, ticketStatusLabel } = loadTsModule("src/features/world-chat/ticketPresentation.ts");

assert.equal(compactDuration("25m29.998555982s"), "25m29s");
assert.equal(compactDuration("1h0m14.232882426s"), "1h0m");
assert.equal(compactDuration("45m20s"), "45m20s");
assert.equal(compactElapsedLabel("25m29.998555982s"), "25m");
assert.equal(compactElapsedLabel("1h0m14.232882426s"), "1h");

const idleDoing = describeDoing({ resident: "jade", status: "idle" });
assert.equal(`${doingPrefix("idle", "她正在", idleDoing.verb)}${idleDoing.verb}`, "没在长测里");
assert.equal(`${doingPrefix("running", "她正在", "在敲字")}在敲字`, "她在敲字");
assert.equal(`${doingPrefix("running", "她正在", "接待来客")}接待来客`, "她正在接待来客");
assert.equal(`${doingPrefix("running", "正在", "在敲字")}在敲字`, "在敲字");
assert.equal(`${doingPrefix("sleeping", "她正在", "睡着")}${describeDoing({ resident: "onyx", status: "sleeping" }).verb}`, "睡着");

assert.equal(ticketStatusLabel("open"), "待处理");
assert.equal(ticketStatusLabel("answered"), "已答复");
assert.equal(priorityLabel("urgent"), "紧急");
assert.equal(interventionStatusLabel("in_progress"), "处理中");
assert.deepEqual(interventionFollowups([
  { kind: "chat", targetId: "chat-1" },
  { kind: "intervention", targetId: "intervention-1" },
  { kind: "ticket", targetId: "ticket-1" },
]).map((item) => item.targetId), ["intervention-1"]);
