import Link from "next/link";

const navigation = [
  { name: "Emails", href: "/emails" },
  { name: "Contacts", href: "/contacts" },
  { name: "Domains", href: "/domains" },
  { name: "API Keys", href: "/api-keys" },
  { name: "Webhooks", href: "/webhooks" },
  { name: "Inbound", href: "/inbound" },
  { name: "Settings", href: "/settings/team" },
];

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="min-h-screen bg-gray-50">
      <nav className="bg-white border-b border-gray-200">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex justify-between h-16">
            <div className="flex">
              <Link
                href="/emails"
                className="flex items-center px-4 text-xl font-bold"
              >
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
          </div>
        </div>
      </nav>
      <main className="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
        {children}
      </main>
    </div>
  );
}
