import { FlaskConical } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { QRISTestLab } from "@/components/qris-test-lab";

export default function QRISTestPage() {
  return <div className="page">
    <PageHeader
      eyebrow="Payment laboratory"
      title="QRIS transaction test"
      description="Upload a merchant static QRIS, generate an amount-bound dynamic code, and keep every test payment in PostgreSQL."
      actions={<span className="lab-mode"><FlaskConical size={16} />Testing mode</span>}
    />
    <QRISTestLab />
  </div>;
}
