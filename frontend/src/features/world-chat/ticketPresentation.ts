import type { FollowupItem, TicketPriority, TicketStatus } from "../../types/domain";

export function ticketStatusLabel(status: TicketStatus): string {
  return status === "closed" ? "已关闭" : status === "answered" ? "已答复" : "待处理";
}

export function priorityLabel(priority: TicketPriority): string {
  return priority === "urgent" ? "紧急" : priority === "high" ? "较高" : priority === "low" ? "较低" : "普通";
}

export function formatTicketTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

export function interventionStatusLabel(status: FollowupItem["status"]): string {
  switch (status) {
    case "planned": return "已计划";
    case "in_progress": return "处理中";
    case "failed": return "处理失败";
    case "rolled_back": return "已回滚";
    case "closed": return "已关闭";
    default: return "需要跟进";
  }
}

export function interventionFollowups(items: FollowupItem[]): FollowupItem[] {
  return items.filter((item) => item.kind === "intervention");
}
