import { QRISControlTable } from "@/components/qris-control-table";
import { api } from "@/lib/api";

export default async function QRISControlPage() {
  const [templates, tenants] = await Promise.all([api.getQRSTemplates(), api.getTenants()]);
  return <QRISControlTable initialTemplates={templates} tenants={tenants} />;
}
