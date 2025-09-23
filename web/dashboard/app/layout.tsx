import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "arrgo Dashboard",
  description: "Cluster visibility for the arrgo media automation platform"
};

export default function RootLayout({
  children
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
