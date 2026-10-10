import React, { useState, useEffect, useCallback } from "react";
import { Plus, Boxes, KeyRound, Clock, Trash2, Copy, Check, Loader2 } from "lucide-react";
import { api } from "@/lib/api";
import { allScopes, scopeDescriptions } from "@/lib/scopes";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
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

function slugify(name) {
  return name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

export default function Applications() {
  const [apps, setApps] = useState([]);
  const [loading, setLoading] = useState(true);
  const [createOpen, setCreateOpen] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const list = await api.apps.list();
      const withTokens = await Promise.all(
        list.map(async (app) => {
          try {
            const tokens = await api.apps.tokens(app.id);
            const active = tokens.filter((t) => !t.revokedAt);
            const scopes = [...new Set(active.flatMap((t) => t.scopes || []))];
            const lastUsed = active.map((t) => t.lastUsedAt).filter(Boolean).sort().pop() || null;
            return { ...app, tokenCount: tokens.length, activeCount: active.length, prefix: active[0]?.tokenPrefix, scopes, lastUsed };
          } catch {
            return { ...app, tokenCount: 0, activeCount: 0, scopes: [], lastUsed: null };
          }
        })
      );
      setApps(withTokens);
    } catch (err) {
      toast({ title: "Failed to load applications", description: err.message, variant: "destructive" });
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const removeApp = async (app) => {
    if (!window.confirm(`Delete application "${app.name}"? Its tokens must be revoked first.`)) return;
    try {
      await api.apps.remove(app.id);
      toast({ title: "Application deleted" });
      load();
    } catch (err) {
      toast({ title: "Delete failed", description: err.message, variant: "destructive" });
    }
  };

  return (
    <div>
      <PageHeader
        title="Applications"
        description="Registered external applications with scoped API access."
        actions={
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="h-3.5 w-3.5" /> Register App
          </Button>
        }
      />

      <div className="grid grid-cols-1 gap-4 p-6 md:grid-cols-2 xl:grid-cols-3">
        {apps.map((app) => (
          <div key={app.id} className="flex flex-col rounded-lg border border-border bg-card p-4">
            <div className="flex items-start justify-between">
              <div className="flex items-center gap-2.5">
                <div className="flex h-9 w-9 items-center justify-center rounded-md bg-primary/10 text-primary">
                  <Boxes className="h-4 w-4" />
                </div>
                <div>
                  <p className="text-sm font-semibold text-foreground">{app.name}</p>
                  <p className="text-xs text-muted-foreground">{app.description || "—"}</p>
                </div>
              </div>
              <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground hover:text-destructive" onClick={() => removeApp(app)}>
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </div>

            <div className="mt-4 flex items-center gap-2 rounded-md border border-border bg-muted/30 px-2.5 py-1.5">
              <KeyRound className="h-3 w-3 text-muted-foreground" />
              <code className="font-mono text-xs text-muted-foreground">
                {app.prefix ? `${app.prefix}…` : "No active tokens"}
              </code>
            </div>

            <div className="mt-3 flex flex-wrap gap-1.5">
              {app.scopes.length > 0 ? (
                app.scopes.map((s) => (
                  <Badge key={s} variant="outline" className="border-border font-mono text-[10px] font-normal text-muted-foreground">{s}</Badge>
                ))
              ) : (
                <span className="text-[11px] text-muted-foreground/60">No scopes granted</span>
              )}
            </div>

            <div className="mt-auto flex items-center justify-between pt-4 text-xs text-muted-foreground">
              <span className="flex items-center gap-1"><Clock className="h-3 w-3" /> {timeAgo(app.lastUsed)}</span>
              <StatusBadge status={app.activeCount > 0 ? "active" : "revoked"} />
            </div>
          </div>
        ))}

        {!loading && apps.length === 0 && (
          <div className="col-span-full rounded-lg border border-dashed border-border py-16 text-center text-sm text-muted-foreground">
            No applications registered yet.
          </div>
        )}
      </div>

      <RegisterAppDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={load} />
    </div>
  );
}

function RegisterAppDialog({ open, onOpenChange, onCreated }) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [scopes, setScopes] = useState(["resources:read"]);
  const [created, setCreated] = useState(null);
  const [copied, setCopied] = useState(false);
  const [saving, setSaving] = useState(false);

  const toggle = (s) => setScopes((prev) => (prev.includes(s) ? prev.filter((x) => x !== s) : [...prev, s]));
  const close = (o) => {
    onOpenChange(o);
    if (!o) {
      setName("");
      setDescription("");
      setScopes(["resources:read"]);
      setCreated(null);
      setCopied(false);
    }
  };

  const create = async () => {
    setSaving(true);
    try {
      const app = await api.apps.create({ name, slug: slugify(name), description });
      const token = await api.apps.createToken(app.id, { name: `${name} token`, scopes });
      setCreated(token.token);
      onCreated?.();
    } catch (err) {
      toast({ title: "Failed to register app", description: err.message, variant: "destructive" });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="sm:max-w-[460px]">
        <DialogHeader>
          <DialogTitle>Register Application</DialogTitle>
          <DialogDescription>Create a scoped API token for an external application.</DialogDescription>
        </DialogHeader>

        {created ? (
          <div className="space-y-3 py-2">
            <div className="rounded-md border border-warning/40 bg-warning/10 p-3">
              <p className="text-xs font-medium text-warning">Copy your token now — it won't be shown again.</p>
            </div>
            <div className="flex items-center gap-2">
              <code className="flex-1 truncate rounded-md border border-border bg-muted/40 px-3 py-2 font-mono text-xs text-foreground">{created}</code>
              <Button
                variant="outline"
                size="icon"
                onClick={() => { navigator.clipboard?.writeText(created); setCopied(true); setTimeout(() => setCopied(false), 1500); }}
              >
                {copied ? <Check className="h-4 w-4 text-success" /> : <Copy className="h-4 w-4" />}
              </Button>
            </div>
          </div>
        ) : (
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="app-name">Application name</Label>
              <Input id="app-name" placeholder="Home Assistant" value={name} onChange={(e) => setName(e.target.value)} />
              {name && <p className="text-[11px] text-muted-foreground">slug: <code className="font-mono">{slugify(name)}</code></p>}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="app-desc">Description</Label>
              <Textarea id="app-desc" rows={2} placeholder="Smart home automation bridge" value={description} onChange={(e) => setDescription(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label>Scopes</Label>
              <div className="space-y-2 rounded-md border border-border p-3">
                {allScopes.map((s) => (
                  <div key={s} className="flex items-center gap-2">
                    <Checkbox checked={scopes.includes(s)} onCheckedChange={() => toggle(s)} id={`scope-${s}`} />
                    <label htmlFor={`scope-${s}`} className="flex-1 font-mono text-xs text-foreground">{s}</label>
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
            <Button onClick={create} disabled={saving || !name || scopes.length === 0}>
              {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <KeyRound className="h-3.5 w-3.5" />} Generate token
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
