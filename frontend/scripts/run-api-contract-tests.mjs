import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import ts from "typescript";

const root = path.resolve(import.meta.dirname, "..");
const filename = path.join(root, "src/lib/api/httpApi.ts");
const source = fs.readFileSync(filename, "utf8");
const compiled = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2022,
  },
  fileName: filename,
}).outputText;
const module = { exports: {} };
const require = (specifier) => {
  if (specifier.endsWith("residentSpeak")) return { fatigueMoodFromLevel: () => "fresh" };
  if (specifier.endsWith("duration")) return { compactDuration: (value) => value };
  return {};
};
new Function("exports", "module", "require", compiled)(module.exports, module, require);

const { HttpArenaConsoleApi } = module.exports;
const requests = [];
const jsonResponse = (value) => ({
  ok: true,
  json: async () => value,
  text: async () => JSON.stringify(value),
});

globalThis.fetch = async (url, init = {}) => {
  requests.push({ url: String(url), init });
  if (url === "/api/summary") {
    return jsonResponse({
      followups: [{
        kind: "host_intervention",
        resident: "onyx",
        target_id: "intervention-1",
        title: "检查服务状态",
        preview: "需要程林确认维护结果",
        status: "in_progress",
        created_at: "2026-07-26T09:00:00Z",
        updated_at: "2026-07-26T10:00:00Z",
        maintenance: { service: "arena-console-server" },
      }],
    });
  }
  if (url === "/api/threads") {
    return jsonResponse([
      { resident: "jade", pending_count: 2, last_preview: "jade latest" },
      { resident: "amber", pending_count: 0 },
      { resident: "onyx", pending_count: 1 },
    ]);
  }
  if (String(url).includes("/messages/jade/thread")) {
    return jsonResponse({
      resident: "jade",
      total: 3,
      has_more: true,
      next_before: "jade-oldest-visible",
      messages: [{
        id: "jade-full-message",
        resident: "jade",
        from: "jade",
        to: "chenglin",
        direction: "resident_to_chenglin",
        body: "完整正文，不是 preview",
        created_at: "2026-07-26T12:00:00Z",
        status: "pending",
        read_at: "2026-07-26T12:01:00Z",
      }],
    });
  }
  if (String(url).includes("/messages/amber/thread")) return jsonResponse({ resident: "amber", messages: [], total: 0, has_more: false });
  if (String(url).includes("/messages/onyx/thread")) return jsonResponse({ resident: "onyx", messages: [], total: 0, has_more: false });
  if (url === "/api/chat") {
    const input = JSON.parse(init.body);
    return jsonResponse({
      id: "chenglin-new",
      resident: input.resident,
      from: "chenglin",
      to: input.resident,
      direction: "chenglin_to_resident",
      body: input.body,
      created_at: "2026-07-26T12:02:00Z",
      status: "delivered",
    });
  }
  if (url === "/api/tickets?limit=100") {
    return jsonResponse([{
      id: "ticket-jade-1",
      resident: "jade",
      title: "Need guidance",
      priority: "high",
      status: "answered",
      created_at: "2026-07-25T10:00:00Z",
      updated_at: "2026-07-26T10:00:00Z",
      last_reply_at: "2026-07-26T10:00:00Z",
      last_preview: "完整回复预览",
      reply_count: 2,
      needs_reply: false,
    }]);
  }
  if (url === "/api/tickets/ticket-jade-1") {
    return jsonResponse({
      id: "ticket-jade-1",
      resident: "jade",
      title: "Need guidance",
      body: "完整的原始单据正文",
      priority: "high",
      status: "answered",
      created_at: "2026-07-25T10:00:00Z",
      updated_at: "2026-07-26T10:00:00Z",
      opened_by: "jade",
      replies: [{ id: "reply-1", from: "chenglin", body: "第一条回复", created_at: "2026-07-26T09:00:00Z" }],
    });
  }
  if (url === "/api/ticket-reply") {
    const input = JSON.parse(init.body);
    return jsonResponse({
      id: input.ticket_id,
      resident: "jade",
      title: "Need guidance",
      body: "完整的原始单据正文",
      priority: "high",
      status: input.close ? "closed" : "answered",
      created_at: "2026-07-25T10:00:00Z",
      updated_at: "2026-07-26T11:00:00Z",
      opened_by: "jade",
      replies: [{ id: "reply-new", from: "chenglin", body: input.body, created_at: "2026-07-26T11:00:00Z" }],
    });
  }
  throw new Error(`unexpected request ${url}`);
};

const api = new HttpArenaConsoleApi({ basePath: "/api" });
const summary = await api.getSummary();
assert.equal(summary.followups[0].kind, "intervention");
assert.equal(summary.followups[0].threadId, undefined);
assert.equal(summary.followups[0].status, "in_progress");
assert.equal(summary.followups[0].title, "检查服务状态");
assert.equal(summary.followups[0].updatedAt, "2026-07-26T10:00:00Z");
assert.deepEqual(summary.followups[0].maintenance, { service: "arena-console-server" });
const threads = await api.listWorldThreads();
assert.deepEqual(threads.map((thread) => thread.threadId), ["chat-jade", "chat-amber", "chat-onyx"]);
assert.equal(threads[0].messages[0].body, "完整正文，不是 preview");
assert.equal(threads[0].messages[0].createdAt, "2026-07-26T12:00:00Z");
assert.equal(threads[0].messages[0].readAt, "2026-07-26T12:01:00Z");
assert.equal(threads[0].nextBefore, "jade-oldest-visible");
assert.equal(threads[0].hasMore, true);

await api.getWorldThreadPage("jade", "jade-oldest-visible", 25);
assert.equal(requests.at(-1).url, "/api/messages/jade/thread?limit=25&before=jade-oldest-visible");

const sent = await api.sendWorldChat({ resident: "jade", body: "连续主动消息", boundary_ack: true });
assert.equal(sent.from, "chenglin");
assert.equal(sent.body, "连续主动消息");
assert.deepEqual(JSON.parse(requests.at(-1).init.body), {
  resident: "jade",
  body: "连续主动消息",
  boundary_ack: true,
});

const tickets = await api.listTickets();
assert.equal(tickets[0].status, "answered");
assert.equal(tickets[0].replyCount, 2);
assert.equal(tickets[0].needsReply, false);
const ticket = await api.getTicket("ticket-jade-1");
assert.equal(ticket.body, "完整的原始单据正文");
assert.equal(ticket.replies[0].body, "第一条回复");
const closed = await api.sendWorldTicketReply({ ticket_id: "ticket-jade-1", body: "处理完了", close: true, boundary_ack: true });
assert.equal(closed.status, "closed");
assert.equal(closed.replies[0].body, "处理完了");
