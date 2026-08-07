import Link from "next/link";
import { BrandMark } from "@/components/brand-mark";

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="auth-shell min-h-screen flex items-center justify-center bg-gray-50">
      <div className="w-full max-w-md p-8">
        <Link href="/" className="auth-brand" aria-label="Sender API home">
          <BrandMark size={32} />
          <span>Sender API</span>
        </Link>
        {children}
        <div className="auth-legal-links">
          <Link href="/privacy">Privacy</Link>
          <Link href="/terms">Terms</Link>
          <Link href="/refunds">Refunds</Link>
        </div>
      </div>
    </div>
  );
}
