import type { Metadata } from "next";
import "./globals.css";
import "./enhancements.css";
import "./login.css";
import "./circular-text.css";
import "./embroidery.css";
import "./embroidery-controls.css";
import "./production-export.css";
import "./admin.css";
export const metadata: Metadata = { title: "PrintStudio — Product Customizer", description: "Design and prepare custom products for print." };
export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) { return <html lang="en"><body>{children}</body></html>; }
