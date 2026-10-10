import React, { useState, useEffect, useCallback } from "react";
import { Link } from "react-router-dom";
import {
  Network,
  Activity,
  Globe,
  Boxes,
  Play,
  Plus,
  ArrowRight,
  AlertTriangle,
  KeyRound,
  CheckCircle2,
  Server,
  RefreshCw,
} from "lucide-react";
import { api } from "@/lib/api";
import PageHeader from "@/components/PageHeader";
import StatCard from "@/components/StatCard";
import StatusBadge from "@/components/StatusBadge";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

function timeAgo(iso) {
  if (!iso) return "—";
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.floor(diff / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

const activityIcons = {
  info: { Icon: Play, cls: "text-success" },
  warning: { Icon: AlertTriangle, cls: "text-warning" },
  error: { Icon: AlertTriangle, cls: "text-destructive" },
};

export default function Dashboard() {
  const [tunnels, setTunnels] = useState([]);
  const [status, setStatus] = useState(null);
  const [dnsStats, setDnsStats] = useState(null);
  const [appCount, setAppCount] = useState(null);
  const [cloudflare, setCloudflare] = useState(null);
  const [activity, setActivity] = useState([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    const [t, s, d, a, l, c] = await Promise.allSettled([
      api.tunnels.list(),
      api.system.status(),
      api.dns.stats(),
      api.apps.list(),
      api.logs.list(30),
      api.system.cloudflare(),
    ]);
    if (t.status === "fulfilled") setTunnels(t.value);
    if (s.status === "fulfilled") setStatus(s.value);
    if (d.status === "fulfilled") setDnsStats(d.value);
    if (a.status === "fulfilled") setAppCount(a.value.length);
    if (l.status === "fulfilled") setActivity(l.value.slice(0, 8));
    if (c.status === "fulfilled") setCloudflare(c.value);
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const running = tunnels.filter((t) => t.status === "running").length;
  const errored = tunnels.filter((t) => t.status === "error").length;
  const recent = tunnels.slice(0, 5);

  return (
    <div>
      <PageHeader
        title="Dashboard"
        description="Infrastructure overview across all tunnels, DNS, and integrations."
        actions={
          <Button variant="outline" size="sm" onClick={load} disabled={loading}>
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} /> Refresh
          </Button>
        }
      />
      <div className="space-y-6 p-6">
        {/* Stat cards */}
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <StatCard icon={Network} label="Total Tunnels" value={loading ? "—" : tunnels.length} accent />
          <StatCard icon={Activity} label="Running" value={loading ? "—" : running} sub={`${errored} in error state`} />
          <StatCard icon={Globe} label="DNS Records" value={loading ? "—" : dnsStats?.records ?? "—"} sub={dnsStats ? `${dnsStats.zones} zones` : undefined} />
          <StatCard icon={Boxes} label="Applications" value={loading ? "—" : appCount ?? "—"} />
        </div>

        <div className="grid grid-cols-1 gap-6 xl:grid-cols-3">
          {/* Recent tunnels */}
          <div className="xl:col-span-2">
            <div className="rounded-lg border border-border bg-card">
              <div className="flex items-center justify-between border-b border-border px-4 py-3">
                <h2 className="text-sm font-semibold text-foreground">Recent Tunnels</h2>
                <Button variant="ghost" size="sm" asChild className="h-7 text-xs text-muted-foreground">
                  <Link to="/tunnels">
                    View all <ArrowRight className="ml-1 h-3 w-3" />
                  </Link>
                </Button>
              </div>
              <Table>
                <TableHeader>
                  <TableRow className="border-border hover:bg-transparent">
                    <TableHead className="h-9 text-xs text-muted-foreground">Tunnel</TableHead>
                    <TableHead className="h-9 text-xs text-muted-foreground">Hostname</TableHead>
                    <TableHead className="h-9 text-xs text-muted-foreground">Status</TableHead>
                    <TableHead className="h-9 text-right text-xs text-muted-foreground">Created</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {recent.map((t) => (
                    <TableRow key={t.id} className="border-border">
                      <TableCell className="py-2.5 text-sm font-medium text-foreground">{t.name}</TableCell>
                      <TableCell className="py-2.5 font-mono text-xs text-muted-foreground">{t.publicHostname || "—"}</TableCell>
                      <TableCell className="py-2.5"><StatusBadge status={t.status} /></TableCell>
                      <TableCell className="py-2.5 text-right text-xs text-muted-foreground">{timeAgo(t.createdAt)}</TableCell>
                    </TableRow>
                  ))}
                  {!loading && recent.length === 0 && (
                    <TableRow className="border-border hover:bg-transparent">
                      <TableCell colSpan={4} className="py-10 text-center text-sm text-muted-foreground">
                        No tunnels yet. Create one from the Tunnels page.
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </div>
          </div>

          {/* Right column: activity + health */}
          <div className="space-y-6">
            {/* Infrastructure health */}
            <div className="rounded-lg border border-border bg-card p-4">
              <h2 className="mb-3 text-sm font-semibold text-foreground">Infrastructure Health</h2>
              <div className="space-y-2.5">
                <HealthRow icon={Server} label="cloudflared daemon" ok={!!status} detail={status ? `${status.aliveRunning ?? 0}/${status.desiredRunning ?? 0} up` : undefined} />
                <HealthRow
                  icon={Network}
                  label="Tunnel connections"
                  ok={status ? !status.drift : false}
                  warn={status?.drift}
                  detail={status ? (status.drift ? "drift detected" : "all healthy") : undefined}
                />
                <HealthRow
                  icon={Globe}
                  label="Cloudflare API"
                  ok={!!cloudflare?.connected}
                  detail={cloudflare?.connected ? "connected" : "not configured"}
                />
                <HealthRow icon={CheckCircle2} label="DNS sync" ok={!!cloudflare?.connected} detail={dnsStats ? `${dnsStats.records} records` : undefined} />
              </div>
            </div>

            {/* Recent activity */}
            <div className="rounded-lg border border-border bg-card">
              <div className="border-b border-border px-4 py-3">
                <h2 className="text-sm font-semibold text-foreground">Recent Activity</h2>
              </div>
              <ScrollArea className="h-[220px]">
                <div className="space-y-1 p-2">
                  {activity.map((a) => {
                    const { Icon, cls } = activityIcons[a.severity] ?? activityIcons.info;
                    return (
                      <div key={a.id} className="flex items-start gap-2.5 rounded-md px-2 py-2 hover:bg-muted/40">
                        <Icon className={`mt-0.5 h-3.5 w-3.5 shrink-0 ${cls}`} />
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-xs leading-snug text-foreground">{a.message}</p>
                          <p className="text-[11px] text-muted-foreground">{timeAgo(a.timestamp)}</p>
                        </div>
                      </div>
                    );
                  })}
                  {!loading && activity.length === 0 && (
                    <p className="px-2 py-6 text-center text-xs text-muted-foreground">No recent activity.</p>
                  )}
                </div>
              </ScrollArea>
            </div>
          </div>
        </div>

        {/* Quick actions */}
        <div className="rounded-lg border border-border bg-card p-4">
          <h2 className="mb-3 text-sm font-semibold text-foreground">Quick Actions</h2>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" asChild>
              <Link to="/tunnels"><Plus className="h-3.5 w-3.5" /> New Tunnel</Link>
            </Button>
            <Button variant="outline" size="sm" asChild>
              <Link to="/dns"><Globe className="h-3.5 w-3.5" /> Add DNS Record</Link>
            </Button>
            <Button variant="outline" size="sm" asChild>
              <Link to="/tokens"><KeyRound className="h-3.5 w-3.5" /> Create API Token</Link>
            </Button>
            <Button variant="outline" size="sm" asChild>
              <Link to="/logs"><Activity className="h-3.5 w-3.5" /> View Logs</Link>
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

function HealthRow({ icon: Icon, label, ok, warn, detail }) {
  return (
    <div className="flex items-center justify-between text-xs">
      <div className="flex items-center gap-2 text-muted-foreground">
        <Icon className="h-3.5 w-3.5" />
        {label}
      </div>
      <div className="flex items-center gap-1.5">
        {detail && <span className="text-muted-foreground/70">{detail}</span>}
        <span className={`h-1.5 w-1.5 rounded-full ${warn ? "bg-warning" : ok ? "bg-success" : "bg-destructive"}`} />
      </div>
    </div>
  );
}
