export default function BillingPage() {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Billing</h1>

      <div className="bg-white shadow rounded-lg p-6">
        <h2 className="text-lg font-medium mb-4">Billing</h2>
        <p className="text-gray-600">
          Billing actions are not configured in this deployment. The active plan is read from the team API.
        </p>
      </div>
    </div>
  );
}
