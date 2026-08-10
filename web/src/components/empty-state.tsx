import { Inbox } from "lucide-react";

export function EmptyState({ title, description }: { title: string; description: string }) {
  return <div className="empty-state"><Inbox size={28} aria-hidden="true" /><strong>{title}</strong><p>{description}</p></div>;
}
