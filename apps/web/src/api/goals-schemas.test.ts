import { describe, expect, it } from 'vitest';

import { goalSchema, publicWidgetSnapshotSchema, widgetProfileSchema } from './goals-schemas';

function baseGoal() {
  return {
    id: 'goal_1', name: 'Followers', kind: 'followers', enabled: true,
    target: 1000, current: 825, baseline: 825, providers: [], accounts: [],
    progressBasisPoints: 8250, completed: false,
    createdAt: '2026-08-16T00:00:00Z', updatedAt: '2026-08-16T00:00:00Z', startedAt: '2026-08-16T00:00:00Z',
    configRevision: 1,
  };
}

function baseWidgetProfile() {
  return {
    id: 'widget_1', goalId: 'goal_1', name: 'Widget', enabled: true, publicSlug: 'a'.repeat(40),
    showCurrent: true, showTarget: true, showPercent: true,
    orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
    backgroundColor: '#00000080', foregroundColor: '#ffffff', fillColor: '#7c3aed', borderColor: '#ffffff33',
    borderRadiusPx: 12, opacity: 1.0,
    createdAt: '2026-08-16T00:00:00Z', updatedAt: '2026-08-16T00:00:00Z',
  };
}

describe('goalSchema', () => {
  it('parses a real followers goal response', () => {
    expect(goalSchema.parse(baseGoal())).toMatchObject({ kind: 'followers', current: 825 });
  });

  it('parses a donation goal with currency', () => {
    const donation = { ...baseGoal(), kind: 'donations', currency: 'USD' };
    expect(goalSchema.parse(donation).currency).toBe('USD');
  });

  it('rejects an unknown kind', () => {
    expect(() => goalSchema.parse({ ...baseGoal(), kind: 'unknown' })).toThrow();
  });
});

describe('widgetProfileSchema', () => {
  it('parses a real widget profile response', () => {
    expect(widgetProfileSchema.parse(baseWidgetProfile())).toMatchObject({ orientation: 'horizontal' });
  });

  it('rejects an unknown orientation', () => {
    expect(() => widgetProfileSchema.parse({ ...baseWidgetProfile(), orientation: 'diagonal' })).toThrow();
  });
});

describe('publicWidgetSnapshotSchema', () => {
  it('parses a real public snapshot and never requires internal ids', () => {
    const snapshot = {
      revision: 1, kind: 'goal', goalKind: 'followers', title: 'Followers',
      current: 825, target: 1000, progressBasisPoints: 8250, completed: false,
      presentation: {
        showCurrent: true, showTarget: true, showPercent: true,
        orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
        backgroundColor: '#00000080', foregroundColor: '#ffffff', fillColor: '#7c3aed', borderColor: '#ffffff33',
        borderRadiusPx: 12, opacity: 1.0,
      },
    };
    const parsed = publicWidgetSnapshotSchema.parse(snapshot);
    expect(parsed.title).toBe('Followers');
    expect('id' in parsed).toBe(false);
  });

  it('rejects a kind other than "goal"', () => {
    const snapshot = { revision: 1, kind: 'ticker', goalKind: 'followers', title: '', current: 0, target: 1, progressBasisPoints: 0, completed: false, presentation: {
      showCurrent: true, showTarget: true, showPercent: true, orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
      backgroundColor: '#000', foregroundColor: '#fff', fillColor: '#fff', borderColor: '#fff', borderRadiusPx: 0, opacity: 1,
    } };
    expect(() => publicWidgetSnapshotSchema.parse(snapshot)).toThrow();
  });
});
