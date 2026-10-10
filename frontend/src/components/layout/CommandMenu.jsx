import React from "react";
import { useNavigate } from "react-router-dom";
import {
  LayoutDashboard,
  Network,
  Globe,
  ScrollText,
  Boxes,
  KeyRound,
  Settings,
} from "lucide-react";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";

const pages = [
  { label: "Dashboard", to: "/", icon: LayoutDashboard, group: "Navigation" },
  { label: "Tunnels", to: "/tunnels", icon: Network, group: "Navigation" },
  { label: "DNS Records", to: "/dns", icon: Globe, group: "Navigation" },
  { label: "Logs", to: "/logs", icon: ScrollText, group: "Navigation" },
  { label: "Applications", to: "/applications", icon: Boxes, group: "Navigation" },
  { label: "API Tokens", to: "/tokens", icon: KeyRound, group: "Navigation" },
  { label: "Settings", to: "/settings", icon: Settings, group: "Navigation" },
];

export default function CommandMenu({ open, onOpenChange }) {
  const navigate = useNavigate();

  const go = (to) => {
    navigate(to);
    onOpenChange(false);
  };

  return (
    <CommandDialog open={open} onOpenChange={onOpenChange}>
      <CommandInput placeholder="Search pages and actions…" />
      <CommandList>
        <CommandEmpty>No results found.</CommandEmpty>
        <CommandGroup heading="Navigation">
          {pages.map((p) => (
            <CommandItem key={p.to} onSelect={() => go(p.to)} value={p.label}>
              <p.icon className="mr-2 h-4 w-4 text-muted-foreground" />
              {p.label}
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}