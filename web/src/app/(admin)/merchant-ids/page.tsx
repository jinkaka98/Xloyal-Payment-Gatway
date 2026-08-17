import { TenantConsole } from "@/components/tenant-console";
import { api } from "@/lib/api";

export default async function TenantIDsPage() {
  const [tenants, merchants, qrisTemplates] = await Promise.all([api.getTenants(), api.getMerchantIDs(), api.getQRSTemplates()]);
  return <TenantConsole initialTenants={tenants} merchants={merchants} qrisTemplates={qrisTemplates} />;
}
