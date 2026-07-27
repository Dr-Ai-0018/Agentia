import { Check, ClipboardList, Send, Wrench } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { arenaApi } from "../../lib/api/client";
import { findForbiddenReplyTerms } from "../../lib/reply";
import type { FollowupItem, ResidentId, TicketPriority, WorldTicket, WorldTicketSummary } from "../../types/domain";
import { residentLabel } from "../residents/residentTheme";
import { formatTicketTime, interventionFollowups, interventionStatusLabel, priorityLabel, ticketStatusLabel } from "./ticketPresentation";

type TicketFilters = {
  resident: "all" | ResidentId;
  status: "all" | "open" | "answered" | "closed";
  priority: "all" | TicketPriority;
};

export function TicketWorkspace({ interventions: interventionItems }: { interventions: FollowupItem[] }) {
  const [mode, setMode] = useState<"tickets" | "interventions">("tickets");
  const [filters, setFilters] = useState<TicketFilters>({ resident: "all", status: "all", priority: "all" });
  const [tickets, setTickets] = useState<WorldTicketSummary[]>([]);
  const [activeID, setActiveID] = useState("");
  const [ticket, setTicket] = useState<WorldTicket | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [body, setBody] = useState("");
  const [boundaryAck, setBoundaryAck] = useState(false);
  const [closeTicket, setCloseTicket] = useState(false);
  const [sending, setSending] = useState(false);
  const forbidden = useMemo(() => findForbiddenReplyTerms(body), [body]);
  const interventions = useMemo(() => interventionFollowups(interventionItems), [interventionItems]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    arenaApi.listTickets({
      resident: filters.resident === "all" ? undefined : filters.resident,
      status: filters.status === "all" ? undefined : filters.status,
      priority: filters.priority === "all" ? undefined : filters.priority,
      limit: 100,
    }).then((items) => {
      if (cancelled) return;
      setTickets(items);
      setActiveID((current) => items.some((item) => item.id === current) ? current : items[0]?.id ?? "");
      setError("");
    }).catch((reason) => {
      if (!cancelled) setError(reason instanceof Error ? reason.message : String(reason));
    }).finally(() => {
      if (!cancelled) setLoading(false);
    });
    return () => { cancelled = true; };
  }, [filters]);

  useEffect(() => {
    let cancelled = false;
    if (!activeID) {
      setTicket(null);
      return;
    }
    arenaApi.getTicket(activeID).then((next) => {
      if (!cancelled) setTicket(next);
    }).catch((reason) => {
      if (!cancelled) setError(reason instanceof Error ? reason.message : String(reason));
    });
    return () => { cancelled = true; };
  }, [activeID]);

  async function submit() {
    if (!ticket || !boundaryAck || !body.trim() || forbidden.length > 0) return;
    setSending(true);
    try {
      const updated = await arenaApi.sendWorldTicketReply({
        ticket_id: ticket.id,
        body,
        close: closeTicket,
        boundary_ack: true,
      });
      setTicket(updated);
      setTickets((current) => current.map((item) => item.id === updated.id ? {
        ...item,
        status: updated.status,
        updatedAt: updated.updatedAt,
        replyCount: updated.replies.length,
        needsReply: updated.status === "open",
        lastPreview: updated.replies.at(-1)?.body ?? item.lastPreview,
      } : item));
      setBody("");
      setBoundaryAck(false);
      setCloseTicket(false);
      setError("");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="ticket-workspace-shell">
      <div className="ticket-workspace__modes" role="tablist" aria-label="单据与房务跟进">
        <button type="button" role="tab" aria-selected={mode === "tickets"} className={mode === "tickets" ? "active" : ""} onClick={() => setMode("tickets")}>
          <ClipboardList size={15} />单据记录
        </button>
        <button type="button" role="tab" aria-selected={mode === "interventions"} className={mode === "interventions" ? "active" : ""} onClick={() => setMode("interventions")}>
          <Wrench size={15} />房务跟进{interventions.length > 0 ? <span>{interventions.length}</span> : null}
        </button>
      </div>
      {mode === "interventions" ? <InterventionWorkspace items={interventions} /> : <div className="ticket-workspace">
      <aside className="ticket-list">
        <div className="ticket-filters">
          <select value={filters.resident} onChange={(event) => setFilters((current) => ({ ...current, resident: event.target.value as TicketFilters["resident"] }))}>
            <option value="all">全部住户</option><option value="jade">Jade</option><option value="amber">Amber</option><option value="onyx">Onyx</option>
          </select>
          <select value={filters.status} onChange={(event) => setFilters((current) => ({ ...current, status: event.target.value as TicketFilters["status"] }))}>
            <option value="all">全部状态</option><option value="open">待处理</option><option value="answered">已答复</option><option value="closed">已关闭</option>
          </select>
          <select value={filters.priority} onChange={(event) => setFilters((current) => ({ ...current, priority: event.target.value as TicketFilters["priority"] }))}>
            <option value="all">全部优先级</option><option value="urgent">紧急</option><option value="high">较高</option><option value="medium">普通</option><option value="low">较低</option>
          </select>
        </div>
        {loading ? <div className="ticket-list__empty">正在加载单据…</div> : null}
        {!loading && tickets.length === 0 ? <div className="ticket-list__empty">这里没有单据</div> : null}
        {tickets.map((item) => (
          <button type="button" key={item.id} className={item.id === activeID ? "active" : ""} onClick={() => setActiveID(item.id)}>
            <span className="ticket-list__badges">
              <span className={`ticket-status ticket-status--${item.status}`}>{ticketStatusLabel(item.status)}</span>
              {item.needsReply ? <span className="ticket-needs-reply">等程林处理</span> : null}
            </span>
            <strong>{item.title}</strong>
            <small>{residentLabel(item.resident)} · {priorityLabel(item.priority)} · {item.replyCount} 次回复</small>
            <time>提出 {formatTicketTime(item.createdAt)} · 更新 {formatTicketTime(item.updatedAt)}</time>
            <p>{item.lastPreview}</p>
          </button>
        ))}
      </aside>
      <section className="ticket-detail">
        {!ticket ? <div className="ticket-detail__empty">选择一条单据查看完整内容</div> : (
          <>
            <header className="ticket-detail__head">
              <div>
                <span>{residentLabel(ticket.resident)} · {priorityLabel(ticket.priority)}</span>
                <h2>{ticket.title}</h2>
                <p>提出 {formatTicketTime(ticket.createdAt)} · 更新 {formatTicketTime(ticket.updatedAt)}</p>
              </div>
              <div className="ticket-detail__status">
                <span className={`ticket-status ticket-status--${ticket.status}`}>{ticketStatusLabel(ticket.status)}</span>
                {ticket.status === "open" ? <span className="ticket-needs-reply">等程林处理</span> : null}
              </div>
            </header>
            <div className="ticket-detail__timeline">
              <TicketEntry author={residentLabel(ticket.resident)} body={ticket.body} createdAt={ticket.createdAt} />
              {ticket.replies.map((reply) => <TicketEntry key={reply.id} author={reply.from === "chenglin" ? "程林" : residentLabel(ticket.resident)} body={reply.body} createdAt={reply.createdAt} />)}
            </div>
            {ticket.status !== "closed" ? (
              <div className="ticket-reply">
                <textarea rows={4} value={body} onChange={(event) => { setBody(event.target.value); setBoundaryAck(false); }} placeholder="回复这条正式事项…" />
                {forbidden.length > 0 ? <div className="reply-warning">这些词不能带进她们的世界：{forbidden.join("、")}</div> : null}
                {error ? <div className="reply-composer__error">{error}</div> : null}
                <div className="ticket-reply__footer">
                  <label><input type="checkbox" checked={boundaryAck} onChange={(event) => setBoundaryAck(event.target.checked)} /><Check size={13} />只说世界内信息</label>
                  <label><input type="checkbox" checked={closeTicket} onChange={(event) => setCloseTicket(event.target.checked)} />回复后关闭</label>
                  <button type="button" className="primary-button" disabled={!body.trim() || !boundaryAck || forbidden.length > 0 || sending} onClick={submit}><Send size={15} />{sending ? "发送中" : "回复"}</button>
                </div>
              </div>
            ) : <div className="ticket-detail__closed">这条单据已经关闭，历史仍完整保留。</div>}
          </>
        )}
      </section>
    </div>}
    </div>
  );
}

function InterventionWorkspace({ items }: { items: FollowupItem[] }) {
  return (
    <section className="intervention-workspace">
      <header className="intervention-workspace__head">
        <div><h2>房务跟进</h2><p>这里单独承接系统维护和人工干预，不把它们伪装成聊天或单据。</p></div>
        <span>{items.length} 项待跟进</span>
      </header>
      {items.length === 0 ? <div className="intervention-workspace__empty">当前没有需要跟进的房务事项。</div> : (
        <div className="intervention-list">
          {items.map((item) => (
            <article key={item.targetId} className="intervention-item">
              <header>
                <div><strong>{item.title || "未命名房务事项"}</strong><span>{residentLabel(item.resident)}</span></div>
                <span className={`intervention-status intervention-status--${item.status}`}>{interventionStatusLabel(item.status)}</span>
              </header>
              <p>{item.preview || "没有补充说明。"}</p>
              <dl>
                <div><dt>事项 ID</dt><dd>{item.targetId}</dd></div>
                <div><dt>提出</dt><dd>{formatTicketTime(item.createdAt)}</dd></div>
                <div><dt>更新</dt><dd>{formatTicketTime(item.updatedAt ?? item.createdAt)}</dd></div>
              </dl>
              {item.maintenance && Object.keys(item.maintenance).length > 0 ? (
                <div className="intervention-maintenance">
                  {Object.entries(item.maintenance).map(([key, value]) => <span key={key}>{key}: {value}</span>)}
                </div>
              ) : null}
              <footer>处理动作仍走受控房务流程；状态变化会在下一次同步后更新到这里。</footer>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}

function TicketEntry({ author, body, createdAt }: { author: string; body: string; createdAt: string }) {
  return <article className="ticket-entry"><header><strong>{author}</strong><time>{formatTicketTime(createdAt)}</time></header><p>{body}</p></article>;
}
