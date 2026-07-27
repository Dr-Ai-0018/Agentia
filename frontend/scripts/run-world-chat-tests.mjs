import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { createServer } from "vite";

const root = path.resolve(import.meta.dirname, "..");
const server = await createServer({ root, server: { middlewareMode: true }, appType: "custom" });

try {
  const { WorldMarkdown } = await server.ssrLoadModule("/src/features/world-chat/WorldMarkdown.tsx");
  const { ReplyComposer } = await server.ssrLoadModule("/src/features/world-chat/ReplyComposer.tsx");
  const React = await import("react");

  const body = "## 完整标题\n\n**加粗**与[链接](https://example.com/a/very/long/path)\n\n- 第一项\n- 第二项\n\n```sh\necho complete\n```\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\n<b>不能执行但必须显示的 HTML</b>\n\n![远程图](https://tracker.invalid/pixel.png)";
  const markdown = renderToStaticMarkup(React.createElement(WorldMarkdown, { body }));
  assert.match(markdown, /<h2>完整标题<\/h2>/);
  assert.match(markdown, /<strong>加粗<\/strong>/);
  assert.match(markdown, /target="_blank"/);
  assert.match(markdown, /rel="noreferrer noopener"/);
  assert.match(markdown, /<ul>/);
  assert.match(markdown, /<pre><code class="language-sh">echo complete/);
  assert.match(markdown, /world-markdown__table-wrap/);
  assert.doesNotMatch(markdown, /\snode=/);
  assert.doesNotMatch(markdown, /<b>/);
  assert.match(markdown, /&lt;b&gt;不能执行但必须显示的 HTML&lt;\/b&gt;/);
  assert.doesNotMatch(markdown, /<img/);
  assert.match(markdown, /\[图片：远程图\] https:\/\/tracker\.invalid\/pixel\.png/);

  const targetBody = "不能省略：" + "很长的中英混合正文 / https://example.com/really/long/path ".repeat(20);
  const composer = renderToStaticMarkup(React.createElement(ReplyComposer, {
    draft: { body: "", boundaryAck: false },
    replyTarget: {
      id: "message-1",
      resident: "onyx",
      from: "onyx",
      to: "chenglin",
      body: targetBody,
      createdAt: "2026-07-27T00:00:00Z",
      status: "pending",
    },
    isSending: false,
    onDraftChange: () => {},
    onCancelReply: () => {},
    onSubmit: () => {},
  }));
  assert.ok(composer.includes(targetBody), "reply target must contain the complete source body");
  assert.match(composer, /aria-label="取消回复这条"/);

  const css = fs.readFileSync(path.join(root, "src/styles/globals.css"), "utf8");
  const targetRules = css.match(/\.reply-composer__target[\s\S]*?\.reply-composer__error/)?.[0] ?? "";
  assert.match(targetRules, /overflow-wrap:\s*anywhere/);
  assert.match(targetRules, /min-width:\s*0/);
  assert.doesNotMatch(targetRules, /text-overflow:\s*ellipsis/);
  assert.doesNotMatch(targetRules, /white-space:\s*nowrap/);
} finally {
  await server.close();
}
