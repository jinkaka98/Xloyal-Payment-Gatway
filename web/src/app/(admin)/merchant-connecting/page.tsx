import { MerchantConnectionConsole } from "@/components/merchant-connection-console";
import { PageHeader } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { api } from "@/lib/api";
import { formatDate, formatRelativeTime } from "@/lib/format";
import type { MerchantID } from "@/lib/types";

export default async function MerchantConnectingPage() {
  let merchants: MerchantID[] = [];
  let unavailable = false;
  try {
    merchants = await api.getMerchantIDs();
  } catch {
    unavailable = true;
  }
  const connections = await Promise.all(merchants.map((merchant) => api.getMerchantConnection(merchant.id)));
  const nekoURL = process.env.NEKO_PUBLIC_URL?.trim() || "";
  return <div className="page"><PageHeader eyebrow="QRIS Control / Machine Checker" title="Merchant Connecting" description="Pantau identitas koneksi, lifecycle Camoufox, dan sinkronisasi transaksi InterActive QRIS." />{unavailable && <p className="connection-notice error">Schema Merchant ID belum tersedia di database lokal. Jalankan migration connector sebelum membuat atau menghubungkan Merchant ID.</p>}<MerchantConnectionConsole merchants={merchants} connections={connections} nekoURL={nekoURL} /><section className="section-block"><div className="section-heading"><div><span className="section-kicker">Connection ledger</span><h2>Connected Merchant IDs</h2><p>Identitas dan status aktual connector browser yang dikelola backend.</p></div></div><div className="table-scroll"><table><thead><tr><th>Merchant</th><th>Internal ID</th><th>InterActive ID</th><th>Browser profile</th><th>Dibuat</th><th>API</th><th>Lifecycle</th><th>Koneksi terakhir</th><th>Sync terakhir</th><th>Hasil worker</th></tr></thead><tbody>{merchants.map((merchant,index) => { const connection = connections[index]; return <tr key={merchant.id}><td><strong>{merchant.name}</strong></td><td><code>{merchant.id}</code></td><td><code>{merchant.interactive_merchant_id}</code></td><td><code>{merchant.id}</code><span className="cell-subtitle">browser-session</span></td><td>{formatDate(merchant.created_at)}</td><td><StatusBadge status={merchant.active ? "active" : "inactive"} /></td><td><StatusBadge status={connection?.status ?? "disconnected"} /></td><td>{connection?.updated_at && new Date(connection.updated_at).getFullYear() > 2000 ? formatRelativeTime(connection.updated_at) : "Belum pernah"}</td><td>{connection?.last_synced_at ? formatRelativeTime(connection.last_synced_at) : "Belum pernah"}</td><td className="worker-result">{connection?.last_error || "Tidak ada error."}</td></tr>; })}</tbody></table></div></section></div>;
}
