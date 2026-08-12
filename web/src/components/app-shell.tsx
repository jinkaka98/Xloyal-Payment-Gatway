"use client";

import { useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Cable, FileClock, FlaskConical, LayoutDashboard, LogOut, Menu, QrCode, ReceiptText, Settings2, UsersRound, X } from "lucide-react";

const nav = [{ label: "Merchant", items: [{ href: "/merchant-ids", label: "Tenant ID", icon: UsersRound }, { href: "/merchant-transactions", label: "Transaksi Merchant", icon: ReceiptText }] }, { label: "QRIS Control", items: [{ href: "/qris-control", label: "QRIS Control", icon: QrCode }, { href: "/qris-test", label: "QRIS Test Transaksi", icon: FlaskConical }, { href: "/merchant-connecting", label: "Merchant Connecting", icon: Cable }, { href: "/global-transactions", label: "Global Log Transaksi", icon: FileClock }, { href: "/health", label: "System Health", icon: Settings2 }] }];

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
        <nav aria-label="Primary navigation"><Link href="/dashboard" className={pathname === "/dashboard" ? "nav-link active" : "nav-link"}><LayoutDashboard size={18} /><span>Dashboard QRIS</span></Link>
          {nav.map((group) => <div key={group.label} className="nav-group"><span>{group.label}</span>{group.items.map(({ href, label, icon: Icon }) => { const active = pathname === href || pathname.startsWith(`${href}/`); return <Link key={href} href={href} className={active ? "nav-link active" : "nav-link"} onClick={() => setOpen(false)} aria-current={active ? "page" : undefined}><Icon size={18} /><span>{label}</span></Link>; })}</div>)}
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
