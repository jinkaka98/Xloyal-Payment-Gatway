import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = { title: { default: "Xloyal Admin", template: "%s | Xloyal Admin" }, description: "QRIS payment operations console" };
export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) { return <html lang="en"><body>{children}</body></html>; }
