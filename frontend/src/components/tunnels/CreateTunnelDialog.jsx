import React, { useState, useEffect } from "react";
import { Globe, ArrowRight, Loader2 } from "lucide-react";
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
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { api } from "@/lib/api";
import { toast } from "@/components/ui/use-toast";

export default function CreateTunnelDialog({ open, onOpenChange, onCreated }) {
  const [zones, setZones] = useState([]);
  const [name, setName] = useState("");
  const [zone, setZone] = useState("");
  const [subdomain, setSubdomain] = useState("");
  const [service, setService] = useState("http://localhost:8080");
  const [autoStart, setAutoStart] = useState(true);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    api.dns
      .zones()
      .then((z) => {
        setZones(z);
        setZone((prev) => prev || z[0]?.id || "");
      })
      .catch(() => setZones([]));
  }, [open]);

  const selectedZone = zones.find((z) => z.id === zone);
  const preview = subdomain && selectedZone ? `${subdomain}.${selectedZone.name}` : "<subdomain>.zone";

  const reset = () => {
    setName("");
    setSubdomain("");
    setService("http://localhost:8080");
    setAutoStart(true);
  };

  const create = async () => {
    setSaving(true);
    try {
      await api.tunnels.create({
        name,
        zone_id: zone,
        domain: selectedZone?.name || "",
        subdomain,
        address: service,
        auto_start: autoStart,
      });
      toast({ title: "Tunnel created", description: name });
      onOpenChange(false);
      reset();
      onCreated?.();
    } catch (err) {
      toast({ title: "Failed to create tunnel", description: err.message, variant: "destructive" });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(o) => { onOpenChange(o); if (!o) reset(); }}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>Create Tunnel</DialogTitle>
          <DialogDescription>Expose a local service through a Cloudflare Tunnel.</DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="space-y-1.5">
            <Label htmlFor="t-name">Tunnel name</Label>
            <Input id="t-name" placeholder="edge-prod-us" value={name} onChange={(e) => setName(e.target.value)} />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="t-zone">Cloudflare zone</Label>
              <Select value={zone} onValueChange={setZone}>
                <SelectTrigger id="t-zone"><SelectValue placeholder="Select zone" /></SelectTrigger>
                <SelectContent>
                  {zones.map((z) => <SelectItem key={z.id} value={z.id}>{z.name}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="t-sub">Subdomain</Label>
              <Input id="t-sub" placeholder="edge" value={subdomain} onChange={(e) => setSubdomain(e.target.value)} />
            </div>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="t-svc">Local service URL</Label>
            <Input id="t-svc" placeholder="http://localhost:8080" value={service} onChange={(e) => setService(e.target.value)} className="font-mono text-xs" />
          </div>

          <div className="flex items-center justify-between rounded-md border border-border bg-muted/30 px-3 py-2.5">
            <div className="space-y-0.5">
              <Label htmlFor="t-auto" className="text-xs font-medium">Auto-start</Label>
              <p className="text-[11px] text-muted-foreground">Start this tunnel when the daemon launches</p>
            </div>
            <Switch id="t-auto" checked={autoStart} onCheckedChange={setAutoStart} />
          </div>

          {/* Hostname preview */}
          <div className="rounded-md border border-border bg-background px-3 py-2.5">
            <p className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Public hostname</p>
            <div className="flex items-center gap-2 font-mono text-xs">
              <span className="text-muted-foreground">{service}</span>
              <ArrowRight className="h-3 w-3 text-muted-foreground/60" />
              <span className="font-medium text-primary">{preview}</span>
              <Globe className="h-3 w-3 text-muted-foreground/60" />
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={create} disabled={saving || !name || !subdomain || !zone}>
            {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Globe className="h-3.5 w-3.5" />} Create tunnel
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
