import React, { useState, useEffect, useCallback } from "react";
import { Plus, KeyRound, Copy, Check, Trash2, Loader2 } from "lucide-react";
import { api } from "@/lib/api";
import { allScopes, scopeDescriptions } from "@/lib/scopes";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { toast } from "@/components/ui/use-toast";

function timeAgo(iso) {
  if (!iso) return "never";
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.floor(diff / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

function fmtDate(iso) {
  if (!iso) return "—";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleDateString("en-CA");
}

export default function ApiTokens() {
  const [tokens, setTokens] = useState([]);
  const [loading, setLoading] = useState(true);
  const [createOpen, setCreateOpen] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setTokens(await api.tokens.list());
    } catch (err) {
      toast({ title: "Failed to load tokens", description: err.message, variant: "destructive" });
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const revoke = async (token) => {
    if (!window.confirm(`Revoke token "${token.name}"?`)) return;
    try {
      await api.apps.revokeToken(token.appId, token.id);
      toast({ title: "Token revoked" });
      load();
    } catch (err) {
      toast({ title: "Revoke failed", description: err.message, variant: "destructive" });
    }
  };

  const purge = async (token) => {
    if (!window.confirm(`Permanently delete token "${token.name}"? This cannot be undone.`)) return;
    try {
      await api.apps.purgeToken(token.appId, token.id);
      toast({ title: "Token deleted" });
      load();
    } catch (err) {
      toast({ title: "Delete failed", description: err.message, variant: "destructive" });
    }
  };

  return (
    <div>
      <PageHeader
        title="API Tokens"
        description="Scoped tokens in the cft_app_ format for programmatic access."
        actions={
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="h-3.5 w-3.5" /> Create Token
          </Button>
        }
      />

      <div className="p-6">
        <div className="overflow-hidden rounded-lg border border-border bg-card">
          <Table>
            <TableHeader>
              <TableRow className="border-border hover:bg-transparent">
                <TableHead className="h-9 text-xs text-muted-foreground">Name</TableHead>
                <TableHead className="h-9 text-xs text-muted-foreground">Application</TableHead>
                <TableHead className="h-9 text-xs text-muted-foreground">Token</TableHead>
                <TableHead className="h-9 text-xs text-muted-foreground">Scopes</TableHead>
                <TableHead className="h-9 text-xs text-muted-foreground">Created</TableHead>
                <TableHead className="h-9 text-xs text-muted-foreground">Last used</TableHead>
                <TableHead className="h-9 text-xs text-muted-foreground">Status</TableHead>
                <TableHead className="h-9 w-24 text-right text-xs text-muted-foreground" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.map((t) => (
                <TableRow key={`${t.appId}-${t.id}`} className="border-border">
                  <TableCell className="py-2.5 text-sm font-medium text-foreground">{t.name}</TableCell>
                  <TableCell className="py-2.5 text-xs text-muted-foreground">{t.appName}</TableCell>
                  <TableCell className="py-2.5">
                    <code className="font-mono text-xs text-muted-foreground">{t.prefix}…</code>
                  </TableCell>
                  <TableCell className="py-2.5">
                    <div className="flex flex-wrap gap-1">
                      {t.scopes.map((s) => (
                        <Badge key={s} variant="outline" className="border-border font-mono text-[10px] font-normal text-muted-foreground">{s}</Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell className="py-2.5 text-xs text-muted-foreground tabular-nums">{fmtDate(t.created)}</TableCell>
                  <TableCell className="py-2.5 text-xs text-muted-foreground">{timeAgo(t.lastUsed)}</TableCell>
                  <TableCell className="py-2.5"><StatusBadge status={t.status} /></TableCell>
                  <TableCell className="py-2.5 text-right">
                    <div className="flex justify-end gap-0.5">
                      {t.revoked ? (
                        <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground hover:text-destructive" title="Delete permanently" onClick={() => purge(t)}>
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      ) : (
                        <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground hover:text-destructive" title="Revoke" onClick={() => revoke(t)}>
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {!loading && tokens.length === 0 && (
                <TableRow className="border-border hover:bg-transparent">
                  <TableCell colSpan={8} className="py-10 text-center text-sm text-muted-foreground">
                    No tokens yet. Register an application to issue one.
                  </TableCell>
                </TableRow>
              )}
              {loading && tokens.length === 0 && (
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

      <CreateTokenDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={load} />
    </div>
  );
}

function CreateTokenDialog({ open, onOpenChange, onCreated }) {
  const [apps, setApps] = useState([]);
  const [appId, setAppId] = useState("");
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState(["resources:read"]);
  const [created, setCreated] = useState(null);
  const [copied, setCopied] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    api.apps
      .list()
      .then((list) => {
        setApps(list);
        setAppId((prev) => prev || (list[0] ? String(list[0].id) : ""));
      })
      .catch(() => setApps([]));
  }, [open]);

  const toggle = (s) => setScopes((prev) => (prev.includes(s) ? prev.filter((x) => x !== s) : [...prev, s]));
  const close = (o) => {
    onOpenChange(o);
    if (!o) {
      setName("");
      setScopes(["resources:read"]);
      setCreated(null);
      setCopied(false);
    }
  };

  const create = async () => {
    setSaving(true);
    try {
      const token = await api.apps.createToken(appId, { name, scopes });
      setCreated(token.token);
      onCreated?.();
    } catch (err) {
      toast({ title: "Failed to create token", description: err.message, variant: "destructive" });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="sm:max-w-[460px]">
        <DialogHeader>
          <DialogTitle>Create API Token</DialogTitle>
          <DialogDescription>Generate a scoped token using the cft_app_ format.</DialogDescription>
        </DialogHeader>

        {created ? (
          <div className="space-y-3 py-2">
            <div className="rounded-md border border-warning/40 bg-warning/10 p-3">
              <p className="text-xs font-medium text-warning">Copy your token now — it won't be shown again.</p>
            </div>
            <div className="flex items-center gap-2">
              <code className="flex-1 truncate rounded-md border border-border bg-muted/40 px-3 py-2 font-mono text-xs text-foreground">{created}</code>
              <Button variant="outline" size="icon" onClick={() => { navigator.clipboard?.writeText(created); setCopied(true); setTimeout(() => setCopied(false), 1500); }}>
                {copied ? <Check className="h-4 w-4 text-success" /> : <Copy className="h-4 w-4" />}
              </Button>
            </div>
          </div>
        ) : (
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label>Application</Label>
              <Select value={appId} onValueChange={setAppId}>
                <SelectTrigger><SelectValue placeholder="Select application" /></SelectTrigger>
                <SelectContent>
                  {apps.map((a) => <SelectItem key={a.id} value={String(a.id)}>{a.name}</SelectItem>)}
                </SelectContent>
              </Select>
              {apps.length === 0 && <p className="text-[11px] text-warning">Register an application first.</p>}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="tok-name">Token name</Label>
              <Input id="tok-name" placeholder="CI Deploy Bot" value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label>Scopes</Label>
              <div className="space-y-2 rounded-md border border-border p-3">
                {allScopes.map((s) => (
                  <div key={s} className="flex items-center gap-2">
                    <Checkbox checked={scopes.includes(s)} onCheckedChange={() => toggle(s)} id={`tk-${s}`} />
                    <label htmlFor={`tk-${s}`} className="flex-1 font-mono text-xs text-foreground">{s}</label>
                    <span className="text-[11px] text-muted-foreground">{scopeDescriptions[s]}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => close(false)}>{created ? "Done" : "Cancel"}</Button>
          {!created && (
            <Button onClick={create} disabled={saving || !appId || !name || scopes.length === 0}>
              {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <KeyRound className="h-3.5 w-3.5" />} Generate token
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
