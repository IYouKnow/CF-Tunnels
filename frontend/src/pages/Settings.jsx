import React, { useState, useEffect, useCallback } from "react";
import {
  Palette,
  Moon,
  Sun,
  Info,
  DownloadCloud,
  CheckCircle2,
  Cloud,
  ShieldCheck,
  LogOut,
  ExternalLink,
  Loader2,
  RefreshCw,
} from "lucide-react";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/AuthContext";
import PageHeader from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";
import { toast } from "@/components/ui/use-toast";

export default function Settings() {
  const { user, logout } = useAuth();
  const [info, setInfo] = useState(null);
  const [cloudflare, setCloudflare] = useState(null);
  const [appUpdate, setAppUpdate] = useState(null);
  const [cfUpdate, setCfUpdate] = useState(null);
  const [busy, setBusy] = useState({});
  const [theme, setTheme] = useState("dark");

  const load = useCallback(async () => {
    const [versions, cf] = await Promise.allSettled([api.system.versions(), api.system.cloudflare()]);
    if (versions.status === "fulfilled") setInfo(versions.value);
    if (cf.status === "fulfilled") setCloudflare(cf.value);
  }, []);

  useEffect(() => {
    load();
    const stored = localStorage.getItem("cft-theme");
    if (stored) setTheme(stored);
  }, [load]);

  useEffect(() => {
    const root = document.documentElement;
    if (theme === "light") root.classList.add("light");
    else root.classList.remove("light");
    localStorage.setItem("cft-theme", theme);
  }, [theme]);

  const withBusy = async (key, fn, successTitle) => {
    setBusy((b) => ({ ...b, [key]: true }));
    try {
      const res = await fn();
      toast({ title: successTitle, description: res?.message });
      return res;
    } catch (err) {
      toast({ title: "Action failed", description: err.message, variant: "destructive" });
      return null;
    } finally {
      setBusy((b) => ({ ...b, [key]: false }));
    }
  };

  return (
    <div>
      <PageHeader title="Settings" description="System configuration and environment information." />

      <div className="max-w-3xl space-y-5 p-6">
        {/* Appearance */}
        <Section icon={Palette} title="Appearance" desc="Theme preference is stored locally in your browser.">
          <div className="flex items-center justify-between py-1">
            <div className="flex items-center gap-2">
              {theme === "dark" ? <Moon className="h-4 w-4 text-muted-foreground" /> : <Sun className="h-4 w-4 text-muted-foreground" />}
              <span className="text-sm font-medium text-foreground">{theme === "dark" ? "Dark" : "Light"} mode</span>
            </div>
            <Switch checked={theme === "light"} onCheckedChange={(v) => setTheme(v ? "light" : "dark")} />
          </div>
        </Section>

        {/* Versions */}
        <Section icon={Info} title="Versions" desc="Installed software versions and update status.">
          <Row label="CF Tunnels" value={info ? `v${info.appVersion}` : "…"} />
          <Row label="cloudflared" value={info?.cloudflaredVersion || "…"} mono />
        </Section>

        {/* App update */}
        <Section icon={DownloadCloud} title="Application update" desc="Check GitHub releases and update the app binary.">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <span className="text-sm text-foreground">
                {appUpdate ? (appUpdate.hasUpdate ? `Update available: v${appUpdate.latestVersion}` : "Up to date") : "Not checked"}
              </span>
              {appUpdate?.hasUpdate && <Badge className="border-transparent bg-primary/15 text-primary hover:bg-primary/15">v{appUpdate.latestVersion}</Badge>}
            </div>
            <div className="flex items-center gap-2">
              {appUpdate?.releaseUrl && (
                <Button size="sm" variant="ghost" asChild>
                  <a href={appUpdate.releaseUrl} target="_blank" rel="noreferrer">Release notes <ExternalLink className="ml-1 h-3 w-3" /></a>
                </Button>
              )}
              <Button
                size="sm"
                variant="outline"
                disabled={busy.appCheck}
                onClick={() => withBusy("appCheck", () => api.system.checkAppUpdate().then(setAppUpdate), "Update check complete")}
              >
                {busy.appCheck ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />} Check
              </Button>
              <Button
                size="sm"
                disabled={busy.appUpdate || !appUpdate?.hasUpdate || info?.docker}
                onClick={() => withBusy("appUpdate", api.system.updateApp, "Update started")}
                title={info?.docker ? "Self-update is disabled in Docker — pull a new image" : undefined}
              >
                {busy.appUpdate ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <DownloadCloud className="h-3.5 w-3.5" />} Update
              </Button>
            </div>
          </div>
        </Section>

        {/* cloudflared update */}
        <Section icon={DownloadCloud} title="cloudflared" desc="Update the cloudflared binary.">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <span className="text-sm text-foreground">
              {cfUpdate ? (cfUpdate.hasUpdate ? `Update available: ${cfUpdate.latestVersion}` : "Up to date") : "Not checked"}
            </span>
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant="outline"
                disabled={busy.cfCheck}
                onClick={() => withBusy("cfCheck", () => api.system.checkCloudflaredUpdate().then(setCfUpdate), "Update check complete")}
              >
                {busy.cfCheck ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />} Check
              </Button>
              <Button size="sm" disabled={busy.cfUpdate} onClick={() => withBusy("cfUpdate", api.system.updateCloudflared, "cloudflared updated")}>
                {busy.cfUpdate ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <DownloadCloud className="h-3.5 w-3.5" />} Update
              </Button>
            </div>
          </div>
        </Section>

        {/* Cloudflare configuration */}
        <Section icon={Cloud} title="Cloudflare Configuration" desc="Connection status to the Cloudflare API.">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <span className={cn("h-2 w-2 rounded-full", cloudflare?.connected ? "bg-success" : "bg-destructive")} />
              <span className="text-sm text-foreground">{cloudflare?.connected ? "Connected" : "Disconnected"}</span>
            </div>
            <Button size="sm" variant="outline" disabled={busy.cf} onClick={() => withBusy("cf", async () => { const res = await api.system.cloudflare(); setCloudflare(res); return res; }, "Connection tested")}>
              {busy.cf ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null} Test connection
            </Button>
          </div>
          {cloudflare && !cloudflare.connected && cloudflare.error && (
            <p className="mt-2 text-xs text-muted-foreground">{cloudflare.error}</p>
          )}
        </Section>

        {/* Security / session */}
        <Section icon={ShieldCheck} title="Security & Session" desc="Current session and authentication details.">
          <Row label="Signed in as" value={user || "—"} />
          <Row label="Role" value="Administrator" />
          <div className="flex items-center justify-between border-t border-border pt-3">
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <CheckCircle2 className="h-4 w-4" />
              Session cookie is HttpOnly and expires after 7 days
            </div>
            <Button size="sm" variant="outline" onClick={logout}>
              <LogOut className="h-3.5 w-3.5" /> Sign out
            </Button>
          </div>
        </Section>
      </div>
    </div>
  );
}

function Section({ icon: Icon, title, desc, children }) {
  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <div className="mb-3 flex items-start gap-3">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <Icon className="h-4 w-4" />
        </div>
        <div>
          <h2 className="text-sm font-semibold text-foreground">{title}</h2>
          <p className="text-xs text-muted-foreground">{desc}</p>
        </div>
      </div>
      <div className="pl-11">{children}</div>
    </div>
  );
}

function Row({ label, value, mono }) {
  return (
    <div className="flex items-center justify-between py-2">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className={cn("text-sm text-foreground", mono && "font-mono text-xs")}>{value}</span>
    </div>
  );
}
