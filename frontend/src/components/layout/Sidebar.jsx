import React from "react";
import { NavLink, useLocation } from "react-router-dom";
import {
  LayoutDashboard,
  Network,
  Globe,
  ScrollText,
  Boxes,
  KeyRound,
  Settings,
  PanelLeftClose,
  PanelLeft,
  Cloud,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";

const nav = [
  {
    section: "Overview",
    items: [{ label: "Dashboard", to: "/", icon: LayoutDashboard }],
  },
  {
    section: "Infrastructure",
    items: [
      { label: "Tunnels", to: "/tunnels", icon: Network },
      { label: "DNS", to: "/dns", icon: Globe },
      { label: "Logs", to: "/logs", icon: ScrollText },
    ],
  },
  {
    section: "Integrations",
    items: [
      { label: "Applications", to: "/applications", icon: Boxes },
      { label: "API Tokens", to: "/tokens", icon: KeyRound },
    ],
  },
  {
    section: "System",
    items: [{ label: "Settings", to: "/settings", icon: Settings }],
  },
];

export default function Sidebar({ collapsed, onToggle, onNavigate }) {
  const location = useLocation();

  return (
    <aside
      className={cn(
        "flex h-full flex-col border-r border-border bg-sidebar transition-[width] duration-200 ease-out",
        collapsed ? "w-[60px]" : "w-[232px]"
      )}
    >
      {/* Brand */}
      <div className="flex h-14 items-center gap-2.5 border-b border-border px-3.5">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground">
          <Cloud className="h-4 w-4" strokeWidth={2.5} />
        </div>
        {!collapsed && (
          <div className="flex flex-col leading-none">
            <span className="text-[13px] font-semibold tracking-tight text-foreground">CF Tunnels</span>
            <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">Self-hosted</span>
          </div>
        )}
      </div>

      {/* Nav */}
      <nav className="flex-1 overflow-y-auto px-2.5 py-3">
        {nav.map((group) => (
          <div key={group.section} className="mb-5">
            {!collapsed && (
              <p className="mb-1 px-2.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground/70">
                {group.section}
              </p>
            )}
            <div className="space-y-0.5">
              {group.items.map((item) => {
                const active = location.pathname === item.to;
                return (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    end={item.to === "/"}
                    onClick={() => onNavigate?.()}
                    title={collapsed ? item.label : undefined}
                    className={cn(
                      "group flex items-center gap-2.5 rounded-md px-2.5 py-[7px] text-[13px] font-medium transition-colors",
                      collapsed && "justify-center px-0",
                      active
                        ? "bg-sidebar-accent text-sidebar-accent-foreground"
                        : "text-sidebar-foreground hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground"
                    )}
                  >
                    <item.icon
                      className={cn("h-4 w-4 shrink-0", active ? "text-primary" : "text-muted-foreground group-hover:text-foreground")}
                      strokeWidth={2}
                    />
                    {!collapsed && <span className="truncate">{item.label}</span>}
                  </NavLink>
                );
              })}
            </div>
          </div>
        ))}
      </nav>

      {/* Collapse toggle */}
      <div className="border-t border-border p-2.5">
        <Button
          variant="ghost"
          size="sm"
          onClick={onToggle}
          className={cn("w-full justify-start text-muted-foreground hover:text-foreground", collapsed && "justify-center px-0")}
        >
          {collapsed ? <PanelLeft className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
          {!collapsed && <span className="text-xs">Collapse</span>}
        </Button>
      </div>
    </aside>
  );
}