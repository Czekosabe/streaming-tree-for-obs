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
    id: 'widget_1', kind: 'goal', goalId: 'goal_1', name: 'Widget', enabled: true, publicSlug: 'a'.repeat(40),
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

  it('parses a Stage 18B event-derived kind with its own filters and no goalId', () => {
    const { goalId: _goalId, ...withoutGoal } = baseWidgetProfile();
    const parsed = widgetProfileSchema.parse({
      ...withoutGoal, kind: 'latest_donation', providers: ['streamelements'], accounts: ['src_1'], showMessage: true,
    });
    expect(parsed.kind).toBe('latest_donation');
    expect(parsed.providers).toEqual(['streamelements']);
    expect(parsed.goalId).toBeUndefined();
  });

  it('parses a dashboard kind with bounded children', () => {
    const { goalId: _goalId, ...withoutGoal } = baseWidgetProfile();
    const parsed = widgetProfileSchema.parse({
      ...withoutGoal, kind: 'dashboard', columns: 2,
      children: [{ widgetProfileId: 'widget_2', column: 1, columnSpan: 1, row: 1, rowSpan: 1 }],
    });
    expect(parsed.kind).toBe('dashboard');
    expect(parsed.children).toHaveLength(1);
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

  it('rejects a kind outside the closed enum', () => {
    const snapshot = { revision: 1, kind: 'ticker', title: '', presentation: {
      showCurrent: true, showTarget: true, showPercent: true, orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
      backgroundColor: '#000', foregroundColor: '#fff', fillColor: '#fff', borderColor: '#fff', borderRadiusPx: 0, opacity: 1,
    } };
    expect(() => publicWidgetSnapshotSchema.parse(snapshot)).toThrow();
  });

  it('parses a latest_follower snapshot with a real latest item', () => {
    const snapshot = {
      revision: 1, kind: 'latest_follower', title: 'Latest Follower',
      latest: { itemId: 'supitem_1', displayName: 'Ada', provider: 'twitch', observedAt: '2026-08-17T00:00:00Z' },
      presentation: {
        showCurrent: true, showTarget: true, showPercent: true, orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
        backgroundColor: '#000', foregroundColor: '#fff', fillColor: '#fff', borderColor: '#fff', borderRadiusPx: 0, opacity: 1,
      },
    };
    const parsed = publicWidgetSnapshotSchema.parse(snapshot);
    expect(parsed.latest?.displayName).toBe('Ada');
    expect('itemId' in (parsed.latest ?? {})).toBe(true);
  });

  it('parses an event_ticker snapshot with a bounded list of typed items', () => {
    const snapshot = {
      revision: 1, kind: 'event_ticker', title: 'Ticker',
      ticker: [{ itemId: 'supitem_1', eventType: 'follow', observedAt: '2026-08-17T00:00:00Z' }],
      presentation: {
        showCurrent: true, showTarget: true, showPercent: true, orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
        backgroundColor: '#000', foregroundColor: '#fff', fillColor: '#fff', borderColor: '#fff', borderRadiusPx: 0, opacity: 1,
      },
    };
    const parsed = publicWidgetSnapshotSchema.parse(snapshot);
    expect(parsed.ticker).toHaveLength(1);
    expect(parsed.ticker?.[0]?.eventType).toBe('follow');
  });

  it('parses a dashboard snapshot composing a real child, never exposing an internal widget id', () => {
    const snapshot = {
      revision: 1, kind: 'dashboard', title: 'Dashboard',
      dashboard: [
        {
          key: 'dashboard_child_0', column: 1, columnSpan: 1, row: 1, rowSpan: 1,
          snapshot: {
            revision: 1, kind: 'latest_follower', title: 'Latest Follower',
            presentation: {
              showCurrent: true, showTarget: true, showPercent: true, orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
              backgroundColor: '#000', foregroundColor: '#fff', fillColor: '#fff', borderColor: '#fff', borderRadiusPx: 0, opacity: 1,
            },
          },
        },
      ],
      presentation: {
        showCurrent: true, showTarget: true, showPercent: true, columns: 2, orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
        backgroundColor: '#000', foregroundColor: '#fff', fillColor: '#fff', borderColor: '#fff', borderRadiusPx: 0, opacity: 1,
      },
    };
    const parsed = publicWidgetSnapshotSchema.parse(snapshot);
    expect(parsed.dashboard).toHaveLength(1);
    expect(parsed.dashboard?.[0]?.snapshot.kind).toBe('latest_follower');
    expect('widgetProfileId' in (parsed.dashboard?.[0] ?? {})).toBe(false);
  });
});
