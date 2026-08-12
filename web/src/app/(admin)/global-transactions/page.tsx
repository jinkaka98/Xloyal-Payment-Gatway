import { PageHeader } from "@/components/page-header";
import { GlobalTransactionConsole } from "@/components/global-transaction-console";
import { api } from "@/lib/api";
export default async function GlobalTransactionsPage() { const items = await api.getGlobalTransactions(); return <div className="page"><PageHeader eyebrow="QRIS Control" title="Global Log Transaksi" description="Ledger transaksi merchant dan lifecycle validasi request QRIS Test." /><GlobalTransactionConsole initialItems={items} /></div>; }
