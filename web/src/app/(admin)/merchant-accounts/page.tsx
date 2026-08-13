import { MerchantAccountsConsole } from "@/components/merchant-accounts-console";
import { api } from "@/lib/api";

export default async function MerchantAccountsPage() {
  const [items, tenants] = await Promise.all([api.getMerchantAccounts(), api.getTenants()]);
  return <MerchantAccountsConsole initialAccounts={items} tenants={tenants} />;
}
