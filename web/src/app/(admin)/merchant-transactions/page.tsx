import { PageHeader } from "@/components/page-header";
import { TenantTransactionTable } from "@/components/tenant-transaction-table";
import { api } from "@/lib/api";

export default async function MerchantTransactionsPage() {
  const [transactions, tenants] = await Promise.all([api.getTenantTransactions(), api.getTenants()]);
  const tenantNames = Object.fromEntries(tenants.map((tenant) => [tenant.id, tenant.name]));
  return <div className="page">
    <PageHeader eyebrow="Merchant / Tenant requests" title="Transaksi Merchant" description="Request pembayaran dari setiap Tenant ID beserta hasil cross-check saldo dan status invoice." />
    <TenantTransactionTable transactions={transactions} tenants={tenants} tenantNames={tenantNames} />
  </div>;
}
