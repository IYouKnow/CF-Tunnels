import React, { useState, useEffect, useMemo, useRef, useCallback } from "react";
import { useSearchParams } from "react-router-dom";
import { Search, Pause, Play, Radio, Filter } from "lucide-react";
import { api } from "@/lib/api";
import PageHeader from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";

const sevStyles = {
  error: "text-destructive",
  warning: "text-warning",
  info: "text-muted-foreground",
};

function fmtTime(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso || "";
  return d.toLocaleTimeString("en-GB", { hour12: false });
}

function fmtDate(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString("en-CA");
}

export default function Logs() {
  const [searchParams, setSearchParams] = useSearchParams();
  const initialTunnel = searchParams.get("tunnel") || "all";

  const [logs, setLogs] = useState([]);
  const [tunnelNames, setTunnelNames] = useState({});
  const [query, setQuery] = useState("");
  const [tunnel, setTunnel] = useState(initialTunnel);
  const [severity, setSeverity] = useState("all");
  const [paused, setPaused] = useState(false);
  const [live, setLive] = useState(false);
  const pausedRef = useRef(paused);
  pausedRef.current = paused;

  // Tunnel filter options + id -> name lookup.
  useEffect(() => {
    api.tunnels
      .list()
      .then((list) => setTunnelNames(Object.fromEntries(list.map((t) => [String(t.id), t.name]))))
      .catch(() => {});
  }, []);

  // History.
  useEffect(() => {
    api.logs.list(500).then(setLogs).catch(() => {});
  }, []);

  const nameFor = useCallback(
    (id) => (id === null || id === undefined ? "daemon" : tunnelNames[String(id)] || `#${id}`),
    [tunnelNames]
  );

  // Live stream — reconnects whenever the tunnel filter changes.
  useEffect(() => {
    const url = api.logs.streamUrl(tunnel === "all" ? null : tunnel);
    const es = new EventSource(url, { withCredentials: true });
    es.onopen = () => setLive(true);
    es.onerror = () => setLive(false);
    es.onmessage = (event) => {
      if (pausedRef.current) return;
      try {
        const e = JSON.parse(event.data);
        const entry = {
          id: `sse_${e.id}_${e.timestamp}`,
          tunnelId: e.tunnel_id,
          timestamp: e.timestamp,
          severity: e.level,
          message: e.message,
        };
        setLogs((prev) => [entry, ...prev].slice(0, 500));
      } catch {
        // ignore malformed frames
      }
    };
    return () => es.close();
  }, [tunnel]);

  const tunnelOptions = useMemo(() => Object.entries(tunnelNames), [tunnelNames]);

  const filtered = useMemo(() => {
    return logs.filter((l) => {
      const q = query.toLowerCase();
      const name = nameFor(l.tunnelId);
      const m = !q || l.message.toLowerCase().includes(q) || name.toLowerCase().includes(q);
      const t = tunnel === "all" || String(l.tunnelId) === tunnel;
      const s = severity === "all" || l.severity === severity;
      return m && t && s;
    });
  }, [logs, query, tunnel, severity, nameFor]);

  const onTunnelChange = (value) => {
    setTunnel(value);
    setSearchParams(value === "all" ? {} : { tunnel: value }, { replace: true });
  };

  return (
    <div className="flex h-full flex-col">
      <PageHeader title="Logs" description="Live tunnel and daemon logs streamed from cloudflared." />

      {/* Toolbar */}
      <div className="flex flex-col gap-2 border-b border-border px-6 py-3 lg:flex-row lg:items-center">
        <div className="relative flex-1">
          <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input placeholder="Filter logs…" value={query} onChange={(e) => setQuery(e.target.value)} className="h-8 pl-8 font-mono text-xs" />
        </div>
        <Select value={tunnel} onValueChange={onTunnelChange}>
          <SelectTrigger className="h-8 w-full text-xs lg:w-[180px]">
            <Filter className="mr-1.5 h-3.5 w-3.5 text-muted-foreground" />
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All tunnels</SelectItem>
            {tunnelOptions.map(([id, name]) => <SelectItem key={id} value={id}>{name}</SelectItem>)}
          </SelectContent>
        </Select>
        <Select value={severity} onValueChange={setSeverity}>
          <SelectTrigger className="h-8 w-full text-xs lg:w-[140px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All severities</SelectItem>
            <SelectItem value="info">Info</SelectItem>
            <SelectItem value="warning">Warning</SelectItem>
            <SelectItem value="error">Error</SelectItem>
          </SelectContent>
        </Select>
        <Button variant={paused ? "default" : "outline"} size="sm" onClick={() => setPaused((p) => !p)} className="h-8">
          {paused ? <Play className="h-3.5 w-3.5" /> : <Pause className="h-3.5 w-3.5" />}
          {paused ? "Resume" : "Pause"}
        </Button>
      </div>

      {/* Live indicator */}
      <div className="flex items-center justify-between border-b border-border bg-muted/20 px-6 py-2">
        <div className="flex items-center gap-2">
          <span className="relative flex h-2 w-2">
            {live && !paused && <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-success opacity-60" />}
            <span className={cn("relative inline-flex h-2 w-2 rounded-full", paused ? "bg-warning" : live ? "bg-success" : "bg-muted-foreground")} />
          </span>
          <span className="text-xs font-medium text-muted-foreground">
            {paused ? "Updates paused" : live ? "Live (SSE)" : "Disconnected"}
          </span>
          <Radio className="h-3 w-3 text-muted-foreground" />
        </div>
        <span className="text-xs text-muted-foreground tabular-nums">{filtered.length} lines</span>
      </div>

      {/* Log console */}
      <ScrollArea className="flex-1 bg-[hsl(0_0%_3%)]">
        <div className="px-4 py-2 font-mono text-xs leading-relaxed">
          {filtered.map((l) => (
            <div key={l.id} className="flex gap-3 border-b border-border/30 py-1 hover:bg-muted/20">
              <span className="shrink-0 text-muted-foreground/60 tabular-nums">{fmtDate(l.timestamp)} {fmtTime(l.timestamp)}</span>
              <span className={cn("w-16 shrink-0 font-semibold uppercase", sevStyles[l.severity] ?? "text-muted-foreground")}>{l.severity}</span>
              <span className="w-36 shrink-0 truncate text-primary/80">{nameFor(l.tunnelId)}</span>
              <span className="text-foreground/90">{l.message}</span>
            </div>
          ))}
          {filtered.length === 0 && (
            <div className="py-10 text-center text-muted-foreground">No log entries match your filters.</div>
          )}
        </div>
      </ScrollArea>
    </div>
  );
}
