import type { Metadata } from "next";
import { Inter } from "next/font/google";
import "./globals.css";
import { ThemeProvider } from "../components/theme/ThemeProvider";
import { EnvironmentProvider } from "../components/environment/EnvironmentContext";
import { AuthProvider } from "../components/auth/AuthContext";

const inter = Inter({ subsets: ["latin"], display: "swap" });

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
    <html lang="en" data-theme="dark" suppressHydrationWarning>
      <body className={inter.className}>
        <ThemeProvider>
          <EnvironmentProvider>
            <AuthProvider>{children}</AuthProvider>
          </EnvironmentProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
