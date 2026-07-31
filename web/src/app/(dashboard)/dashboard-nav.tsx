"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { clearActiveTeamId } from "@/lib/api";
import { createClient } from "@/lib/supabase/client";

const navigation = [
  { name: "Emails", href: "/emails" },
  { name: "Contacts", href: "/contacts" },
  { name: "Domains", href: "/domains" },
  { name: "API Keys", href: "/api-keys" },
  { name: "Webhooks", href: "/webhooks" },
  { name: "Inbound", href: "/inbound" },
  { name: "Settings", href: "/settings/team" },
  { name: "Billing", href: "/settings/billing" },
];

export function DashboardNav() {
  const router = useRouter();

  const logout = async () => {
    await createClient().auth.signOut();
    clearActiveTeamId();
    router.replace("/login");
    router.refresh();
  };

  return (
    <nav aria-label="Main navigation" className="bg-white border-b border-gray-200">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex justify-between h-16">
          <div className="flex">
            <Link href="/emails" className="flex items-center px-4 text-xl font-bold">
              Sender API
            </Link>
            <div className="hidden sm:ml-6 sm:flex sm:space-x-8">
              {navigation.map((item) => (
                <Link
                  key={item.name}
                  href={item.href}
                  className="inline-flex items-center px-1 pt-1 text-sm font-medium text-gray-900 hover:text-blue-600"
                >
                  {item.name}
                </Link>
              ))}
            </div>
          </div>
          <details className="relative sm:hidden self-center">
            <summary className="cursor-pointer list-none rounded-md px-3 py-2 text-sm font-medium text-gray-700 hover:text-blue-600">
              Menu
            </summary>
            <div className="absolute right-0 z-10 mt-2 w-48 rounded-md border border-gray-200 bg-white p-2 shadow-lg">
              {navigation.map((item) => (
                <Link
                  key={item.name}
                  href={item.href}
                  className="block rounded px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 hover:text-blue-600"
                >
                  {item.name}
                </Link>
              ))}
            </div>
          </details>
          <button
            type="button"
            onClick={() => void logout()}
            className="px-3 text-sm font-medium text-gray-700 hover:text-blue-600"
          >
            Log out
          </button>
        </div>
      </div>
    </nav>
  );
}
