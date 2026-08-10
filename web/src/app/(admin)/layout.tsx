import { AppShell } from "@/components/app-shell";
export const dynamic = "force-dynamic";
export default function AdminLayout({ children }: { children: React.ReactNode }) { return <AppShell>{children}</AppShell>; }
