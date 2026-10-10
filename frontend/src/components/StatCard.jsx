import React from "react";
import { cn } from "@/lib/utils";

export default function StatCard({ icon: Icon, label, value, sub, accent, className }) {
  return (
    <div className={cn("rounded-lg border border-border bg-card p-4 transition-colors hover:border-border/80", className)}>
      <div className="flex items-start justify-between">
        <div className="space-y-2">
          <p className="text-xs font-medium text-muted-foreground">{label}</p>
          <p className="text-2xl font-semibold tracking-tight text-foreground tabular-nums">{value}</p>
          {sub && <p className="text-xs text-muted-foreground">{sub}</p>}
        </div>
        <div className={cn("flex h-8 w-8 items-center justify-center rounded-md", accent ? "bg-primary/10 text-primary" : "bg-muted text-muted-foreground")}>
          <Icon className="h-4 w-4" strokeWidth={2} />
        </div>
      </div>
    </div>
  );
}