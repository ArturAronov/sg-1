import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "SG-1 · Internal studio", template: "%s · SG-1" },
  description: "Your ideas, made useful. The internal application studio for your team.",
  robots: { index: false, follow: false },
  icons: { icon: "/icon.svg" },
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
