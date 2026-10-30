// YouTube client: offline library, subscriptions and downloads over videod.
// OAuth and the real yt-dlp/mpv path remain unconfigured.
import { createPortClient } from "./client-kit";
import type { AppConfig, Metric, Section, SettingRow } from "./model";

const sections: Section[] = [
  {
    id: "library", title: "LIBRARY", jp: "ライブラリ", items: [
      { id: "video-0011", title: "Building a Handheld UI", subtitle: "Pocket Dev · 18:22 · 1080p", meta: "offline", badge: "MP4", progress: 0.36, body: "A build log for the 3.5 inch handheld UI, from layout grid to native view tree.", lines: [{ label: "views", value: "184k" }, { label: "size", value: "212 MB" }, { label: "codec", value: "h264" }] },
      { id: "video-0024", title: "Night Market Walk", subtitle: "Slow Travel · 42:10 · 720p", meta: "offline", badge: "MP4", progress: 0, body: "A single-take walk through a night market." },
      { id: "video-0037", title: "Synth Repair, Part 3", subtitle: "Bench Notes · 26:04 · 1080p", meta: "queued", badge: "QUE", progress: 0.12, body: "Replacing the voice chips and recalibrating the filter." },
    ],
  },
  {
    id: "channels", title: "SUBSCRIPTIONS", jp: "登録チャンネル", items: [
      { id: "ch-1", title: "Pocket Dev", subtitle: "24 videos · 8 offline", meta: "subscribed", badge: "CH", body: "Runtime, native contract and device builds." },
      { id: "ch-2", title: "Slow Travel", subtitle: "112 videos · 3 offline", meta: "subscribed", badge: "CH", body: "Long-form city and harbour walks." },
      { id: "ch-3", title: "Downloads", subtitle: "2 running · 1 paused", meta: "yt-dlp", badge: "DWN", body: "Download queue and completed archive." },
    ],
  },
];

const metrics: Metric[] = [
  { label: "VIDEOS", value: "1,208" },
  { label: "OFFLINE", value: "64" },
  { label: "AUTH", value: "mock" },
];

const settings: SettingRow[] = [
  { id: "quality", label: "Quality", jp: "画質", value: "1080P", options: ["1080P", "720P", "480P"] },
  { id: "audio", label: "Audio only", jp: "音声のみ", value: "OFF", options: ["OFF", "ON"] },
  { id: "account", label: "Account", jp: "アカウント", value: "DEMO", options: ["DEMO", "NONE"] },
];

export const APP: AppConfig = {
  title: "YOUTUBE",
  jp: "動画",
  kind: "video",
  page: "YOUTUBE",
  tabs: ["ライブラリ", "再生", "設定"],
  actions: ["PREV", "PLAY", "NEXT", "SAVE"],
  settingsTitle: "ACCOUNT",
  settingsJp: "アカウント設定",
  deviceGap: "videod + yt-dlp/OAuth missing.",
  sourceNote: "/api/library · /api/downloads",
};

export const client = createPortClient({
  resource: "youtube",
  fixture: { sections, metrics, settings },
  endpoints: {
    list: "/api/library",
    get: (id) => `/api/library/${encodeURIComponent(id)}`,
    action: (name) => `/api/${name === "SAVE" ? "downloads" : "player"}/${encodeURIComponent(name.toLowerCase())}`,
  },
  act(name, payload, bundle) {
    const items = bundle.sections.flatMap((section) => section.items);
    const item = items.find((entry) => entry.id === payload?.id) ?? items[0];
    if (!item) return { ok: false, message: "no video selected" };
    if (name === "PLAY") { item.state = item.state === "playing" ? "paused" : "playing"; item.progress = Math.min(1, (item.progress ?? 0) + 0.04); return { ok: true, message: `${item.title} ${item.state} · mock player` }; }
    if (name === "NEXT" || name === "PREV") {
      const at = items.findIndex((entry) => entry.id === item.id);
      const next = items[(at + (name === "NEXT" ? 1 : items.length - 1)) % items.length]!;
      return { ok: true, message: `${name}: ${next.title}` };
    }
    if (name === "SAVE") { item.badge = "SAV"; return { ok: true, message: `${item.title} queued for offline save` }; }
    return { ok: false, message: `${name} unavailable` };
  },
});
