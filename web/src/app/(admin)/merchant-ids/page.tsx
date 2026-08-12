import { TenantConsole } from "@/components/tenant-console";
import { api } from "@/lib/api";

export default async function TenantIDsPage() {
  const [tenants, merchants] = await Promise.all([api.getTenants(), api.getMerchantIDs()]);
  return <TenantConsole initialTenants={tenants} merchants={merchants} />;
}
