import type { ParseKeys } from 'i18next';
import {
  Activity,
  FileText,
  LayoutDashboard,
  MessageSquare,
  MonitorPlay,
  Radio,
  Settings,
  SlidersHorizontal,
  Tv,
  type LucideIcon,
} from 'lucide-react';

export type NavItem = {
  to: string;
  /** Key in the `navigation` namespace; resolved when the item renders. */
  labelKey: ParseKeys<'navigation'>;
  icon: LucideIcon;
  /** Marks routes that are still empty placeholder views. */
  planned: boolean;
};

export const NAV_ITEMS: readonly NavItem[] = [
  { to: '/', labelKey: 'items.dashboard', icon: LayoutDashboard, planned: false },
  { to: '/platforms', labelKey: 'items.platforms', icon: Tv, planned: true },
  { to: '/streams', labelKey: 'items.streams', icon: Radio, planned: true },
  { to: '/metadata', labelKey: 'items.metadata', icon: SlidersHorizontal, planned: true },
  { to: '/engagement', labelKey: 'items.engagement', icon: Activity, planned: false },
  { to: '/chat', labelKey: 'items.chat', icon: MessageSquare, planned: false },
  { to: '/overlays', labelKey: 'items.overlays', icon: MonitorPlay, planned: false },
  { to: '/settings', labelKey: 'items.settings', icon: Settings, planned: true },
  { to: '/logs', labelKey: 'items.logs', icon: FileText, planned: true },
];
