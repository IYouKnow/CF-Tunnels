import React from "react";
import { useLocation } from "react-router-dom";
import { Search, ChevronRight, LogOut, Settings as SettingsIcon, ChevronsUpDown, Menu } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useAuth } from "@/lib/AuthContext";

const titles = {
  "/": "Dashboard",
  "/tunnels": "Tunnels",
  "/dns": "DNS Records",
  "/logs": "Logs",
  "/applications": "Applications",
  "/tokens": "API Tokens",
  "/settings": "Settings",
};

export default function Topbar({ onOpenSearch, onOpenMobile }) {
  const location = useLocation();
  const { user, logout } = useAuth();
  const title = titles[location.pathname] ?? "CF Tunnels";
  const initials = (user || "admin").slice(0, 2).toUpperCase();

  return (
    <header className="flex h-14 shrink-0 items-center gap-3 border-b border-border bg-background/80 px-4 backdrop-blur-sm">
      {/* Mobile menu */}
      <Button variant="ghost" size="icon" onClick={onOpenMobile} className="h-8 w-8 lg:hidden">
        <Menu className="h-4 w-4" />
      </Button>

      {/* Breadcrumb / title */}
      <div className="flex items-center gap-2 text-sm">
        <span className="hidden text-muted-foreground sm:inline">CF Tunnels</span>
        <ChevronRight className="hidden h-3.5 w-3.5 text-muted-foreground/50 sm:inline" />
        <span className="font-medium text-foreground">{title}</span>
      </div>

      {/* Search trigger */}
      <button
        onClick={onOpenSearch}
        className="ml-auto hidden h-8 w-64 items-center gap-2 rounded-md border border-border bg-muted/40 px-3 text-xs text-muted-foreground transition-colors hover:border-border/80 hover:bg-muted/70 sm:flex"
      >
        <Search className="h-3.5 w-3.5" />
        <span>Search…</span>
        <kbd className="ml-auto rounded border border-border bg-background px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">⌘K</kbd>
      </button>

      <Button variant="ghost" size="icon" onClick={onOpenSearch} className="ml-auto h-8 w-8 sm:hidden">
        <Search className="h-4 w-4" />
      </Button>

      {/* System status */}
      <div className="hidden items-center gap-2 md:flex">
        <span className="relative flex h-2 w-2">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-success opacity-60" />
          <span className="relative inline-flex h-2 w-2 rounded-full bg-success" />
        </span>
        <span className="text-xs font-medium text-muted-foreground">All systems operational</span>
      </div>

      {/* User menu */}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" className="h-8 gap-2 px-1.5">
            <div className="flex h-6 w-6 items-center justify-center rounded-full bg-primary/15 text-[11px] font-semibold text-primary">
              {initials}
            </div>
            <span className="hidden text-xs font-medium lg:inline">{user || "admin"}</span>
            <ChevronsUpDown className="h-3.5 w-3.5 text-muted-foreground" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-52">
          <DropdownMenuLabel className="text-xs text-muted-foreground">Signed in as</DropdownMenuLabel>
          <DropdownMenuItem className="flex-col items-start gap-0.5 py-2">
            <span className="text-sm font-medium">{user || "admin"}</span>
            <span className="text-xs text-muted-foreground">Administrator</span>
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem asChild>
            <a href="/settings">
              <SettingsIcon className="mr-2 h-4 w-4" /> Settings
            </a>
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={logout} className="text-destructive focus:text-destructive">
            <LogOut className="mr-2 h-4 w-4" /> Sign out
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </header>
  );
}
