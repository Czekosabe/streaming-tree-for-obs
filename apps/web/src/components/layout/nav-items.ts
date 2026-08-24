import type { ParseKeys } from 'i18next';
import {
  Activity,
  Bell,
  Bot,
  FileText,
  LayoutDashboard,
  MessageSquare,
  MonitorPlay,
  Radio,
  Settings,
  SlidersHorizontal,
  Target,
  Tv,
  Volume2,
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
  { to: '/streams', labelKey: 'items.streams', icon: Radio, planned: false },
  { to: '/metadata', labelKey: 'items.metadata', icon: SlidersHorizontal, planned: true },
  { to: '/engagement', labelKey: 'items.engagement', icon: Activity, planned: false },
  { to: '/chat', labelKey: 'items.chat', icon: MessageSquare, planned: false },
  { to: '/overlays', labelKey: 'items.overlays', icon: MonitorPlay, planned: false },
  { to: '/automation', labelKey: 'items.automation', icon: Bot, planned: false },
  { to: '/alerts', labelKey: 'items.alerts', icon: Bell, planned: false },
  { to: '/audio', labelKey: 'items.audio', icon: Volume2, planned: false },
  { to: '/goals', labelKey: 'items.goals', icon: Target, planned: false },
  { to: '/settings', labelKey: 'items.settings', icon: Settings, planned: false },
  { to: '/logs', labelKey: 'items.logs', icon: FileText, planned: false },
];
