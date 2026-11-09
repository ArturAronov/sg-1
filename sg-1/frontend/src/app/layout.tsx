import type { Metadata } from "next";
import localFont from "next/font/local";
import "./globals.css";

const mono = localFont({
  src: [
    { path: "../../public/fonts/IBMPlexMono-Regular.ttf", weight: "400" },
    { path: "../../public/fonts/IBMPlexMono-Medium.ttf", weight: "500" },
  ],
  variable: "--font-mono",
  display: "swap",
});

export const metadata: Metadata = {
  title: { default: "SG-1 · Internal studio", template: "%s · SG-1" },
  description: "Your ideas, made useful. The internal application studio for your team.",
  robots: { index: false, follow: false },
  icons: { icon: "/icon.svg" },
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={mono.variable}>
      <body>{children}</body>
    </html>
  );
}
