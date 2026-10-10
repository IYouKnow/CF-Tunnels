import React from "react";
import { cn } from "@/lib/utils";

const map = {
  running: { dot: "bg-success", text: "text-success", label: "Running" },
  stopped: { dot: "bg-muted-foreground", text: "text-muted-foreground", label: "Stopped" },
  error: { dot: "bg-destructive", text: "text-destructive", label: "Error" },
  active: { dot: "bg-success", text: "text-success", label: "Active" },
  revoked: { dot: "bg-muted-foreground", text: "text-muted-foreground", label: "Revoked" },
};

export default function StatusBadge({ status, className }) {
  const s = map[status] ?? { dot: "bg-muted-foreground", text: "text-muted-foreground", label: status };
  return (
    <span className={cn("inline-flex items-center gap-1.5 text-xs font-medium", s.text, className)}>
      <span className={cn("h-1.5 w-1.5 rounded-full", s.dot)} />
      {s.label}
    </span>
  );
}
