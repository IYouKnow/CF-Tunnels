import React, { useState, useEffect, useMemo, useCallback } from "react";
import { Link } from "react-router-dom";
import {
  Plus,
  Search,
  Play,
  Square,
  RotateCw,
  Pencil,
  ScrollText,
  Trash2,
  MoreHorizontal,
  ChevronDown,
  ChevronRight,
  Globe,
  Filter,
  RefreshCw,
  Loader2,
} from "lucide-react";
import { api } from "@/lib/api";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import CreateTunnelDialog from "@/components/tunnels/CreateTunnelDialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";
import { toast } from "@/components/ui/use-toast";

export default function Tunnels() {
  const [tunnels, setTunnels] = useState([]);
  const [loading, setLoading] = useState(true);
  const [busyId, setBusyId] = useState(null);
  const [query, setQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [createOpen, setCreateOpen] = useState(false);
  const [editTunnel, setEditTunnel] = useState(null);
  const [expanded, setExpanded] = useState(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setTunnels(await api.tunnels.list());
    } catch (err) {
      toast({ title: "Failed to load tunnels", description: err.message, variant: "destructive" });
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const sync = async () => {
    setLoading(true);
    try {
      const res = await api.tunnels.sync();
      toast({
        title: "Synced with Cloudflare",
        description: `imported ${res.imported}, updated ${res.updated}, pruned ${res.prunedOrphans ?? 0}`,
      });
      await load();
    } catch (err) {
      toast({ title: "Sync failed", description: err.message, variant: "destructive" });
      setLoading(false);
    }
  };

  const runAction = async (id, action, label) => {
    setBusyId(id);
    try {
      await action();
      toast({ title: label });
      await load();
    } catch (err) {
      toast({ title: `${label} failed`, description: err.message, variant: "destructive" });
    } finally {
      setBusyId(null);
    }
  };

  const toggleAutoStart = (tunnel, value) =>
    runAction(tunnel.id, () => api.tunnels.update(tunnel.id, { auto_start: value }), "Auto-start updated");

  const removeTunnel = (tunnel) => {
    if (!window.confirm(`Delete tunnel "${tunnel.name}"? This cannot be undone.`)) return;
    runAction(tunnel.id, () => api.tunnels.remove(tunnel.id), "Tunnel deleted");
  };

  const restart = (tunnel) =>
    runAction(
      tunnel.id,
      async () => {
        await api.tunnels.stop(tunnel.id);
        await api.tunnels.start(tunnel.id);
      },
      "Tunnel restarted"
    );

  const filtered = useMemo(() => {
    return tunnels.filter((t) => {
      const q = query.toLowerCase();
      const matches =
        !q ||
        t.name.toLowerCase().includes(q) ||
        t.publicHostname.toLowerCase().includes(q) ||
        t.localService.toLowerCase().includes(q);
      const s = statusFilter === "all" || t.status === statusFilter;
      return matches && s;
    });
  }, [tunnels, query, statusFilter]);

  return (
    <div>
      <PageHeader
        title="Tunnels"
        description="Manage Cloudflare Tunnels routing public hostnames to local services."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={sync} disabled={loading}>
              <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} /> Sync
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="h-3.5 w-3.5" /> New Tunnel
            </Button>
          </>
        }
      />

      <div className="p-6">
        {/* Toolbar */}
        <div className="mb-4 flex flex-col gap-2 sm:flex-row sm:items-center">
          <div className="relative flex-1 sm:max-w-xs">
            <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="Search tunnels…"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              className="h-8 pl-8 text-xs"
            />
          </div>
          <Select value={statusFilter} onValueChange={setStatusFilter}>
            <SelectTrigger className="h-8 w-[140px] text-xs">
              <Filter className="mr-1.5 h-3.5 w-3.5 text-muted-foreground" />
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All statuses</SelectItem>
              <SelectItem value="running">Running</SelectItem>
              <SelectItem value="stopped">Stopped</SelectItem>
              <SelectItem value="error">Error</SelectItem>
            </SelectContent>
          </Select>
          <span className="text-xs text-muted-foreground">{filtered.length} of {tunnels.length}</span>
        </div>

        {/* Table */}
        <div className="overflow-hidden rounded-lg border border-border bg-card">
          <Table>
            <TableHeader>
              <TableRow className="border-border hover:bg-transparent">
                <TableHead className="h-9 w-8" />
                <TableHead className="h-9 text-xs text-muted-foreground">Name</TableHead>
                <TableHead className="h-9 text-xs text-muted-foreground">Public hostname</TableHead>
                <TableHead className="h-9 text-xs text-muted-foreground">Local service</TableHead>
                <TableHead className="h-9 text-xs text-muted-foreground">Status</TableHead>
                <TableHead className="h-9 text-xs text-muted-foreground">Auto-start</TableHead>
                <TableHead className="h-9 text-xs text-muted-foreground">Routes</TableHead>
                <TableHead className="h-9 w-10 text-right text-xs text-muted-foreground" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((t) => {
                const isOpen = expanded === t.id;
                return (
                  <React.Fragment key={t.id}>
                    <TableRow className="border-border">
                      <TableCell className="py-2.5">
                        <button
                          onClick={() => setExpanded(isOpen ? null : t.id)}
                          className="text-muted-foreground hover:text-foreground"
                        >
                          {isOpen ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                        </button>
                      </TableCell>
                      <TableCell className="py-2.5 text-sm font-medium text-foreground">
                        <span className="flex items-center gap-2">
                          {busyId === t.id && <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />}
                          {t.name}
                        </span>
                      </TableCell>
                      <TableCell className="py-2.5 font-mono text-xs text-muted-foreground">{t.publicHostname || "—"}</TableCell>
                      <TableCell className="py-2.5 font-mono text-xs text-muted-foreground">{t.localService || "—"}</TableCell>
                      <TableCell className="py-2.5"><StatusBadge status={t.status} /></TableCell>
                      <TableCell className="py-2.5">
                        <Switch
                          checked={t.autoStart}
                          disabled={busyId === t.id}
                          onCheckedChange={(v) => toggleAutoStart(t, v)}
                          className="scale-90"
                        />
                      </TableCell>
                      <TableCell className="py-2.5 text-xs text-muted-foreground">
                        {t.extraDomainCount > 0 ? <Badge variant="outline" className="border-border text-[10px]">+{t.extraDomainCount}</Badge> : "—"}
                      </TableCell>
                      <TableCell className="py-2.5 text-right">
                        <RowActions
                          tunnel={t}
                          busy={busyId === t.id}
                          onStart={() => runAction(t.id, () => api.tunnels.start(t.id), "Tunnel started")}
                          onStop={() => runAction(t.id, () => api.tunnels.stop(t.id), "Tunnel stopped")}
                          onRestart={() => restart(t)}
                          onEdit={() => setEditTunnel(t)}
                          onDelete={() => removeTunnel(t)}
                        />
                      </TableCell>
                    </TableRow>
                    {isOpen && (
                      <TableRow className="border-border bg-muted/20 hover:bg-transparent">
                        <TableCell />
                        <TableCell colSpan={7} className="py-3">
                          <IngressRoutes tunnel={t} onChanged={load} />
                        </TableCell>
                      </TableRow>
                    )}
                  </React.Fragment>
                );
              })}
              {filtered.length === 0 && !loading && (
                <TableRow className="border-border hover:bg-transparent">
                  <TableCell colSpan={8} className="py-10 text-center text-sm text-muted-foreground">
                    No tunnels match your filters.
                  </TableCell>
                </TableRow>
              )}
              {loading && filtered.length === 0 && (
                <TableRow className="border-border hover:bg-transparent">
                  <TableCell colSpan={8} className="py-10 text-center text-sm text-muted-foreground">
                    <Loader2 className="mx-auto h-4 w-4 animate-spin" />
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      </div>

      <CreateTunnelDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={load} />
      <EditTunnelDialog tunnel={editTunnel} onOpenChange={(o) => !o && setEditTunnel(null)} onSaved={load} />
    </div>
  );
}

function IngressRoutes({ tunnel, onChanged }) {
  const [rules, setRules] = useState([]);
  const [loading, setLoading] = useState(true);
  const [addOpen, setAddOpen] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setRules(await api.ingress.list(tunnel.id));
    } catch (err) {
      toast({ title: "Failed to load routes", description: err.message, variant: "destructive" });
    } finally {
      setLoading(false);
    }
  }, [tunnel.id]);

  useEffect(() => {
    load();
  }, [load]);

  const removeRule = async (rule) => {
    if (!window.confirm(`Delete route "${rule.hostname}"?`)) return;
    try {
      await api.ingress.remove(rule.id);
      toast({ title: "Route deleted" });
      await load();
      onChanged?.();
    } catch (err) {
      toast({ title: "Delete failed", description: err.message, variant: "destructive" });
    }
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Hostname routes</p>
        <Button variant="ghost" size="sm" className="h-6 text-xs text-muted-foreground" onClick={() => setAddOpen(true)}>
          <Plus className="h-3 w-3" /> Add route
        </Button>
      </div>
      <div className="space-y-1.5">
        <RouteRow hostname={tunnel.publicHostname || "—"} service={tunnel.localService || "—"} path="/" primary />
        {rules.map((r) => (
          <RouteRow key={r.id} hostname={r.hostname} service={r.service} path={r.path || "/"} onRemove={() => removeRule(r)} />
        ))}
        {!loading && rules.length === 0 && (
          <p className="text-xs text-muted-foreground/60">No additional routes configured.</p>
        )}
        {loading && <p className="text-xs text-muted-foreground/60">Loading routes…</p>}
      </div>
      <AddRouteDialog open={addOpen} onOpenChange={setAddOpen} tunnel={tunnel} onAdded={() => { load(); onChanged?.(); }} />
    </div>
  );
}

function RouteRow({ hostname, service, path, primary, onRemove }) {
  return (
    <div className="flex items-center gap-2 rounded-md border border-border bg-background px-3 py-2 font-mono text-xs">
      <Globe className="h-3 w-3 text-muted-foreground" />
      <span className={cn("font-medium", primary ? "text-primary" : "text-foreground")}>{hostname}</span>
      <span className="text-muted-foreground/50">{path}</span>
      <span className="text-muted-foreground/40">→</span>
      <span className="text-muted-foreground">{service}</span>
      {primary ? (
        <Badge variant="outline" className="ml-auto border-border text-[10px] text-muted-foreground">primary</Badge>
      ) : (
        <Button variant="ghost" size="icon" className="ml-auto h-6 w-6 text-muted-foreground hover:text-destructive" onClick={onRemove}>
          <Trash2 className="h-3 w-3" />
        </Button>
      )}
    </div>
  );
}

function AddRouteDialog({ open, onOpenChange, tunnel, onAdded }) {
  const [hostname, setHostname] = useState("");
  const [path, setPath] = useState("/");
  const [service, setService] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open) {
      setHostname("");
      setPath("/");
      setService(tunnel.localService || "");
    }
  }, [open, tunnel.localService]);

  const create = async () => {
    setSaving(true);
    try {
      const res = await api.ingress.create({
        tunnel_id: tunnel.id,
        hostname,
        path,
        service,
      });
      toast({ title: "Route added", description: res.dnsWarning || undefined });
      onOpenChange(false);
      onAdded?.();
    } catch (err) {
      toast({ title: "Failed to add route", description: err.message, variant: "destructive" });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[440px]">
        <DialogHeader>
          <DialogTitle>Add hostname route</DialogTitle>
          <DialogDescription>Route an additional hostname through {tunnel.name}.</DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-1.5">
            <Label>Hostname</Label>
            <Input placeholder="api.acme.io" value={hostname} onChange={(e) => setHostname(e.target.value)} className="font-mono text-xs" />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label>Path</Label>
              <Input value={path} onChange={(e) => setPath(e.target.value)} className="font-mono text-xs" />
            </div>
            <div className="space-y-1.5">
              <Label>Service</Label>
              <Input value={service} onChange={(e) => setService(e.target.value)} className="font-mono text-xs" />
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={create} disabled={saving || !hostname}>
            {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />} Add route
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function EditTunnelDialog({ tunnel, onOpenChange, onSaved }) {
  const [name, setName] = useState("");
  const [autoStart, setAutoStart] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (tunnel) {
      setName(tunnel.name);
      setAutoStart(tunnel.autoStart);
    }
  }, [tunnel]);

  const save = async () => {
    setSaving(true);
    try {
      await api.tunnels.update(tunnel.id, { name, auto_start: autoStart });
      toast({ title: "Tunnel updated" });
      onOpenChange(false);
      onSaved?.();
    } catch (err) {
      toast({ title: "Update failed", description: err.message, variant: "destructive" });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={!!tunnel} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[440px]">
        <DialogHeader>
          <DialogTitle>Edit tunnel</DialogTitle>
          <DialogDescription>{tunnel?.publicHostname || tunnel?.name}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-1.5">
            <Label htmlFor="e-name">Name</Label>
            <Input id="e-name" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="flex items-center justify-between rounded-md border border-border bg-muted/30 px-3 py-2.5">
            <Label htmlFor="e-auto" className="text-xs font-medium">Auto-start</Label>
            <Switch id="e-auto" checked={autoStart} onCheckedChange={setAutoStart} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={save} disabled={saving || !name}>
            {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Pencil className="h-3.5 w-3.5" />} Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function RowActions({ tunnel, busy, onStart, onStop, onRestart, onEdit, onDelete }) {
  const running = tunnel.status === "running";
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className="h-7 w-7" disabled={busy}>
          {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <MoreHorizontal className="h-4 w-4" />}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-40">
        {running ? (
          <DropdownMenuItem onClick={onStop}><Square className="mr-2 h-3.5 w-3.5" /> Stop</DropdownMenuItem>
        ) : (
          <DropdownMenuItem onClick={onStart}><Play className="mr-2 h-3.5 w-3.5" /> Start</DropdownMenuItem>
        )}
        <DropdownMenuItem onClick={onRestart}><RotateCw className="mr-2 h-3.5 w-3.5" /> Restart</DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={onEdit}><Pencil className="mr-2 h-3.5 w-3.5" /> Edit</DropdownMenuItem>
        <DropdownMenuItem asChild>
          <Link to={`/logs?tunnel=${tunnel.id}`}><ScrollText className="mr-2 h-3.5 w-3.5" /> Logs</Link>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={onDelete} className="text-destructive focus:text-destructive">
          <Trash2 className="mr-2 h-3.5 w-3.5" /> Delete
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
