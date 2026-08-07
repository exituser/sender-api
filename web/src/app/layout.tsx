import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Sender API",
  description: "Transactional email infrastructure for product teams.",
  metadataBase: new URL("https://chydo.lol"),
  icons: {
    icon: "/brand-mark.svg",
    shortcut: "/brand-mark.svg",
    apple: "/brand-mark.svg",
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
