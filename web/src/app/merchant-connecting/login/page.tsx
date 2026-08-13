import { MerchantConnectingLogin } from "@/components/merchant-connecting-login";

export default async function MerchantConnectingLoginPage({
  searchParams,
}: {
  searchParams: Promise<{ merchant_id?: string }>;
}) {
  const { merchant_id: merchantID = "" } = await searchParams;

  if (!merchantID) {
    return <main className="merchant-popup-shell"><section className="merchant-popup-panel"><h1>Merchant ID tidak tersedia</h1><p>Tutup popup ini dan mulai ulang koneksi dari halaman Merchant Connecting.</p></section></main>;
  }

  return <MerchantConnectingLogin merchantID={merchantID} />;
}
