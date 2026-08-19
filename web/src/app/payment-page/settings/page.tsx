import { AdminPageHeader } from "@/components/payment-page-admin";

export default function PaymentPageSettings() {
  return (
    <div className="page">
      <AdminPageHeader
        title="Payment Page Settings"
        description="Pengaturan checkout dikelola melalui Custom Web Payment."
      />
      <section className="operational-band">
        <div>
          <p className="eyebrow">CONFIGURATION SOURCE</p>
          <h2>Custom Web Payment</h2>
          <p className="cell-subtitle">
            Gunakan editor tema untuk branding, logo, warna, layout, dan tema
            default. Tidak ada pengaturan lokal yang berpura-pura tersimpan di
            halaman ini.
          </p>
        </div>
        <a className="button button-primary" href="/custom-web-payment">
          Buka Custom Web Payment
        </a>
      </section>
    </div>
  );
}
