import React from 'react'

const colorMap: Record<string, string> = {
  pending: 'bg-amber-100 text-amber-700 border-amber-200',
  approved: 'bg-green-100 text-green-700 border-green-200',
  rejected: 'bg-red-100 text-red-700 border-red-200',
  disabled: 'bg-gray-100 text-gray-500 border-gray-200',
}

interface ApprovalBadgeProps {
  status: 'pending' | 'approved' | 'rejected' | 'disabled'
  className?: string
}

export function ApprovalBadge({ status, className = '' }: ApprovalBadgeProps) {
  return (
    <span
      className={`inline-block rounded-full border px-2.5 py-0.5 text-xs font-medium ${colorMap[status] ?? colorMap.disabled} ${className}`}
    >
      {status}
    </span>
  )
}
