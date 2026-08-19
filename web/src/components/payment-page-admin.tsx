import Link from "next/link";

export function Metric({
  label,
  value,
  tone,
}: {
  label: string;
  value: string | number;
  tone?: string;
}) {
  return (
    <div className={`metric-block ${tone ?? ""}`}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

export function AdminPageHeader({
  eyebrow = "PAYMENT PAGE",
  title,
  description,
  action,
}: {
  eyebrow?: string;
  title: string;
  description: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="page-header">
      <div>
        <p className="eyebrow">{eyebrow}</p>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      {action && <div className="page-actions">{action}</div>}
    </div>
  );
}

export function QuickAction({
  href,
  label,
  detail,
}: {
  href: string;
  label: string;
  detail: string;
}) {
  return (
    <Link href={href} className="quick-action">
      <strong>{label}</strong>
      <span>{detail}</span>
    </Link>
  );
}

export function NotImplemented({ children }: { children: React.ReactNode }) {
  return (
    <div className="notice notice-warning">
      <strong>NOT IMPLEMENTED</strong>
      <span>{children}</span>
    </div>
  );
}
