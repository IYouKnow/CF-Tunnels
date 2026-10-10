import React, { useState, useEffect, useMemo, useCallback } from "react";
import { Plus, Search, Pencil, Trash2, Globe, Filter, Loader2 } from "lucide-react";
import { api } from "@/lib/api";
import PageHeader from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Switch } from "@/components/ui/switch";
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

const RECORD_TYPES = ["A", "AAAA", "CNAME", "MX", "TXT"];

const typeColors = {
  A: "text-chart-3 border-chart-3/30 bg-chart-3/10",
  AAAA: "text-chart-2 border-chart-2/30 bg-chart-2/10",
  CNAME: "text-primary border-primary/30 bg-primary/10",
  MX: "text-chart-4 border-chart-4/30 bg-chart-4/10",
  TXT: "text-chart-5 border-chart-5/30 bg-chart-5/10",
};

export default function DNS() {
  const [zones, setZones] = useState([]);
  const [zone, setZone] = useState("");
  const [records, setRecords] = useState([]);
  const [loading, setLoading] = useState(true);
  const [query, setQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState("all");
  const [dialog, setDialog] = useState(null); // { mode, record? }

  const loadZones = useCallback(async () => {
    try {
      const z = await api.dns.zones();
      setZones(z);
      setZone((prev) => prev || z[0]?.id || "");
    } catch (err) {
      toast({ title: "Failed to load zones", description: err.message, variant: "destructive" });
    }
  }, []);

  const loadRecords = useCallback(async () => {
    if (!zone) return;
    setLoading(true);
    try {
      const { records: r } = await api.dns.records(zone);
      setRecords(r);
    } catch (err) {
      toast({ title: "Failed to load DNS records", description: err.message, variant: "destructive" });
      setRecords([]);
    } finally {
      setLoading(false);
    }
  }, [zone]);

  useEffect(() => {
    loadZones();
  }, [loadZones]);

  useEffect(() => {
    loadRecords();
  }, [loadRecords]);

  const removeRecord = async (record) => {
    if (!window.confirm(`Delete record "${record.name}"?`)) return;
    try {
      await api.dns.remove(zone, record.id);
      toast({ title: "Record deleted" });
      loadRecords();
    } catch (err) {
      toast({ title: "Delete failed", description: err.message, variant: "destructive" });
    }
  };

  const filtered = useMemo(() => {
    return records.filter((r) => {
      const q = query.toLowerCase();
      const m = !q || r.name.toLowerCase().includes(q) || r.content.toLowerCase().includes(q);
      const t = typeFilter === "all" || r.type === typeFilter;
      return m && t;
    });
  }, [records, query, typeFilter]);

  return (
    <div>
      <PageHeader
        title="DNS Records"
        description="Manage Cloudflare DNS records across your zones."
        actions={
          <Button size="sm" onClick={() => setDialog({ mode: "add" })} disabled={!zone}>
            <Plus className="h-3.5 w-3.5" /> Add Record
          </Button>
        }
      />

      <div className="p-6">
        {/* Toolbar */}
        <div className="mb-4 flex flex-col gap-2 lg:flex-row lg:items-center">
          <Select value={zone} onValueChange={setZone}>
            <SelectTrigger className="h-8 w-full text-xs lg:w-[220px]">
              <Globe className="mr-1.5 h-3.5 w-3.5 text-muted-foreground" />
              <SelectValue placeholder="Select zone" />
            </SelectTrigger>
            <SelectContent>
              {zones.map((z) => (
                <SelectItem key={z.id} value={z.id}>{z.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="relative flex-1">
            <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input placeholder="Search records…" value={query} onChange={(e) => setQuery(e.target.value)} className="h-8 pl-8 text-xs" />
          </div>
          <Select value={typeFilter} onValueChange={setTypeFilter}>
            <SelectTrigger className="h-8 w-[140px] text-xs">
              <Filter className="mr-1.5 h-3.5 w-3.5 text-muted-foreground" />
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All types</SelectItem>
              {RECORD_TYPES.map((t) => <SelectItem key={t} value={t}>{t}</SelectItem>)}
            </SelectContent>
          </Select>
          <span className="text-xs text-muted-foreground">{filtered.length} records</span>
        </div>

        {/* Table */}
        <div className="overflow-hidden rounded-lg border border-border bg-card">
          <Table>
            <TableHeader>
              <TableRow className="border-border hover:bg-transparent">
                <TableHead className="h-9 w-20 text-xs text-muted-foreground">Type</TableHead>
                <TableHead className="h-9 text-xs text-muted-foreground">Name</TableHead>
                <TableHead className="h-9 text-xs text-muted-foreground">Content</TableHead>
                <TableHead className="h-9 text-xs text-muted-foreground">TTL</TableHead>
                <TableHead className="h-9 text-xs text-muted-foreground">Proxy</TableHead>
                <TableHead className="h-9 w-20 text-right text-xs text-muted-foreground" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((r) => (
                <TableRow key={r.id} className="border-border">
                  <TableCell className="py-2.5">
                    <span className={cn("inline-flex rounded border px-1.5 py-0.5 font-mono text-[10px] font-semibold", typeColors[r.type] ?? "text-muted-foreground border-border bg-muted")}>
                      {r.type}
                    </span>
                  </TableCell>
                  <TableCell className="py-2.5 font-mono text-xs font-medium text-foreground">{r.name}</TableCell>
                  <TableCell className="py-2.5 font-mono text-xs text-muted-foreground">{r.content}</TableCell>
                  <TableCell className="py-2.5 text-xs text-muted-foreground tabular-nums">{!r.ttl || r.ttl === 1 ? "Auto" : `${r.ttl}s`}</TableCell>
                  <TableCell className="py-2.5">
                    {r.proxied ? (
                      <Badge className="border-transparent bg-primary/15 text-primary hover:bg-primary/15"><Globe className="mr-1 h-3 w-3" />Proxied</Badge>
                    ) : (
                      <span className="text-xs text-muted-foreground">DNS only</span>
                    )}
                  </TableCell>
                  <TableCell className="py-2.5 text-right">
                    <div className="flex justify-end gap-0.5">
                      <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground hover:text-foreground" onClick={() => setDialog({ mode: "edit", record: r })}>
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground hover:text-destructive" onClick={() => removeRecord(r)}>
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {filtered.length === 0 && (
                <TableRow className="border-border hover:bg-transparent">
                  <TableCell colSpan={6} className="py-10 text-center text-sm text-muted-foreground">
                    {loading ? <Loader2 className="mx-auto h-4 w-4 animate-spin" /> : "No records found."}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      </div>

      <RecordDialog
        state={dialog}
        zoneId={zone}
        onOpenChange={(o) => !o && setDialog(null)}
        onSaved={loadRecords}
      />
    </div>
  );
}

function RecordDialog({ state, zoneId, onOpenChange, onSaved }) {
  const open = !!state;
  const editing = state?.mode === "edit";
  const [type, setType] = useState("A");
  const [name, setName] = useState("");
  const [content, setContent] = useState("");
  const [proxied, setProxied] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!state) return;
    setType(state.record?.type || "A");
    setName(state.record?.name || "");
    setContent(state.record?.content || "");
    setProxied(state.record?.proxied || false);
  }, [state]);

  const save = async () => {
    setSaving(true);
    try {
      if (editing) {
        await api.dns.update(zoneId, state.record.id, { type, name, content, proxied });
        toast({ title: "Record updated" });
      } else {
        await api.dns.create({ zone_id: zoneId, type, name, content, proxied });
        toast({ title: "Record created" });
      }
      onOpenChange(false);
      onSaved?.();
    } catch (err) {
      toast({ title: "Save failed", description: err.message, variant: "destructive" });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[460px]">
        <DialogHeader>
          <DialogTitle>{editing ? "Edit DNS Record" : "Add DNS Record"}</DialogTitle>
          <DialogDescription>{editing ? "Update this record." : "Create a new record in the selected zone."}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label>Type</Label>
              <Select value={type} onValueChange={setType}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {RECORD_TYPES.map((t) => <SelectItem key={t} value={t}>{t}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label>Name</Label>
              <Input placeholder="edge.example.com" value={name} onChange={(e) => setName(e.target.value)} className="font-mono text-xs" />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label>Content</Label>
            <Input placeholder="198.51.100.12" value={content} onChange={(e) => setContent(e.target.value)} className="font-mono text-xs" />
          </div>
          <div className="flex items-center justify-between rounded-md border border-border bg-muted/30 px-3 py-2.5">
            <div>
              <Label className="text-xs font-medium">Proxied</Label>
              <p className="text-[11px] text-muted-foreground">Route through Cloudflare's proxy</p>
            </div>
            <Switch checked={proxied} onCheckedChange={setProxied} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={save} disabled={saving || !name || !content}>
            {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />} {editing ? "Save" : "Add record"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
