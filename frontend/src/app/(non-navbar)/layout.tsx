import RQProvider from "@/components/provider/RQProvider";
import type { Metadata } from "next";
import { Inter } from "next/font/google";
import "../globals.css";

const inter = Inter({ subsets: ["latin"] });

export const metadata: Metadata = {
  title: "Create Next App",
  description: "Generated by create next app",
};

const RootLayout = ({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) => {
  return (
    <html lang="ko">
      <body className={inter.className}>
        <RQProvider>{children}</RQProvider>
      </body>
    </html>
  );
};

export default RootLayout;
