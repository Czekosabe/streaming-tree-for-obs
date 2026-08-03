import {
  FileText,
  LayoutDashboard,
  Radio,
  Settings,
  SlidersHorizontal,
  Tv,
  type LucideIcon,
} from 'lucide-react';

export type NavItem = {
  to: string;
  label: string;
  icon: LucideIcon;
  /** Marks routes that are still empty placeholder views. */
  planned: boolean;
};

export const NAV_ITEMS: readonly NavItem[] = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard, planned: false },
  { to: '/platforms', label: 'Platforms', icon: Tv, planned: true },
  { to: '/streams', label: 'Streams', icon: Radio, planned: true },
  { to: '/metadata', label: 'Metadata', icon: SlidersHorizontal, planned: true },
  { to: '/settings', label: 'Settings', icon: Settings, planned: true },
  { to: '/logs', label: 'Logs', icon: FileText, planned: true },
];
