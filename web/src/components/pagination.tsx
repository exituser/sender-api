"use client";

interface PaginationProps {
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
  disabled?: boolean;
}

export function Pagination({ page, pageSize, total, onPageChange, disabled = false }: PaginationProps) {
  const pageCount = Math.ceil(total / pageSize);
  if (pageCount <= 1) {
    return null;
  }

  return (
    <nav aria-label="Pagination" className="flex items-center justify-between border-t border-gray-200 px-4 py-3 sm:px-6">
      <p className="text-sm text-gray-700">
        Page <span className="font-medium">{page + 1}</span> of <span className="font-medium">{pageCount}</span>
      </p>
      <div className="flex gap-2">
        <button
          type="button"
          disabled={disabled || page === 0}
          onClick={() => onPageChange(page - 1)}
          className="rounded-md border px-3 py-1 text-sm disabled:opacity-50"
        >
          Previous
        </button>
        <button
          type="button"
          disabled={disabled || page >= pageCount - 1}
          onClick={() => onPageChange(page + 1)}
          className="rounded-md border px-3 py-1 text-sm disabled:opacity-50"
        >
          Next
        </button>
      </div>
    </nav>
  );
}
