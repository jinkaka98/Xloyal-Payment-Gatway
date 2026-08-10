"use client";

import { useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Activity, Building2, FileClock, FileText, FlaskConical, LayoutDashboard, LogOut, Menu, Store, X } from "lucide-react";

const nav = [
  { href: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { href: "/tenants", label: "Tenants", icon: Building2 },
  { href: "/merchant-accounts", label: "Merchant accounts", icon: Store },
  { href: "/invoices", label: "Invoices", icon: FileText },
  { href: "/qris-test", label: "QRIS test lab", icon: FlaskConical },
  { href: "/audit-logs", label: "Audit logs", icon: FileClock },
  { href: "/health", label: "System health", icon: Activity },
];

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const [open, setOpen] = useState(false);
  return (
    <div className="app-shell">
      <button className="mobile-menu icon-button" aria-label="Open navigation" onClick={() => setOpen(true)}><Menu size={20} /></button>
      {open && <button className="sidebar-scrim" aria-label="Close navigation" onClick={() => setOpen(false)} />}
      <aside className={`sidebar ${open ? "sidebar-open" : ""}`}>
        <div className="brand-row">
          <div className="brand-mark" aria-hidden="true">X</div>
          <div><strong>Xloyal</strong><span>Payment operations</span></div>
          <button className="sidebar-close icon-button" aria-label="Close navigation" onClick={() => setOpen(false)}><X size={18} /></button>
        </div>
        <nav aria-label="Primary navigation">
          {nav.map(({ href, label, icon: Icon }) => {
            const active = pathname === href || pathname.startsWith(`${href}/`);
            return <Link key={href} href={href} className={active ? "nav-link active" : "nav-link"} onClick={() => setOpen(false)} aria-current={active ? "page" : undefined}><Icon size={18} /><span>{label}</span></Link>;
          })}
        </nav>
        <div className="sidebar-account">
          <div className="avatar">AR</div>
          <div><strong>Admin Role</strong><span>admin@xloyal.id</span></div>
          <form action="/api/logout" method="post"><button className="icon-button" aria-label="Sign out"><LogOut size={17} /></button></form>
        </div>
      </aside>
      <main className="main-content">{children}</main>
    </div>
  );
}
